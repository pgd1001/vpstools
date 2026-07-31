package main

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pgd1001/svrtools/packages/config"
)

func sqliteTestConfig(t *testing.T) config.BackendConfig {
	t.Helper()
	return config.BackendConfig{
		DatabaseDriver: "sqlite",
		DatabaseURL:    filepath.Join(t.TempDir(), "metadata.db"),
	}
}

// SQLite in WAL mode allows many concurrent readers alongside the single
// writer. The read pool must therefore be wider than the write pool, otherwise
// every list query queues behind runner polling.
func TestReadPoolIsWiderThanTheWritePool(t *testing.T) {
	cfg := sqliteTestConfig(t)
	writeDB, err := openMetadataDatabase(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open write pool: %v", err)
	}
	defer writeDB.Close()
	readDB, err := openMetadataReadDatabase(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	if readDB == nil {
		t.Fatal("sqlite should have a dedicated read pool")
	}
	defer readDB.Close()

	if got := writeDB.Stats().MaxOpenConnections; got != sqliteWritePoolSize {
		t.Fatalf("write pool size = %d, want %d", got, sqliteWritePoolSize)
	}
	if got := readDB.Stats().MaxOpenConnections; got <= sqliteWritePoolSize {
		t.Fatalf("read pool size = %d, want more than the write pool (%d)", got, sqliteWritePoolSize)
	}
}

// PostgreSQL pools connections itself, so it must not get a second handle.
func TestPostgresHasNoSeparateReadPool(t *testing.T) {
	readDB, err := openMetadataReadDatabase(context.Background(), config.BackendConfig{
		DatabaseDriver: "postgres",
		DatabaseURL:    "postgresql://user:secret@localhost:5432/metadata?sslmode=disable",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if readDB != nil {
		readDB.Close()
		t.Fatal("postgres should not open a second pool")
	}
}

// This is the property the split exists for: a read must complete while a write
// transaction is open. With a single shared connection the read would block
// until the writer committed, which is what serialised the whole API.
func TestReadsProceedWhileAWriteTransactionIsOpen(t *testing.T) {
	cfg := sqliteTestConfig(t)
	writeDB, err := openMetadataDatabase(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open write pool: %v", err)
	}
	defer writeDB.Close()
	ctx := context.Background()
	if err := migrate(ctx, writeDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := seed(ctx, writeDB); err != nil {
		t.Fatalf("seed: %v", err)
	}
	readDB, err := openMetadataReadDatabase(ctx, cfg)
	if err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	defer readDB.Close()

	// Hold a write transaction open for longer than the read should take.
	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO servers (id, organisation_id, name, environment, status)
		 VALUES ('srv_pending','org_demo','pending-server','development','active')`); err != nil {
		t.Fatalf("write inside transaction: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		var count int
		done <- readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&count)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("read during open write transaction failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("read blocked behind an open write transaction; the pools are not independent")
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// A committed write must be visible to the read pool, so the split does not
	// trade throughput for stale reads.
	var count int
	if err := readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers WHERE id = 'srv_pending'`).Scan(&count); err != nil {
		t.Fatalf("read after commit: %v", err)
	}
	if count != 1 {
		t.Fatalf("committed write not visible to the read pool (count=%d)", count)
	}
}

// Concurrent readers must genuinely overlap rather than serialise.
func TestConcurrentReadsDoNotSerialise(t *testing.T) {
	cfg := sqliteTestConfig(t)
	writeDB, err := openMetadataDatabase(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open write pool: %v", err)
	}
	defer writeDB.Close()
	ctx := context.Background()
	if err := migrate(ctx, writeDB); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := seed(ctx, writeDB); err != nil {
		t.Fatalf("seed: %v", err)
	}
	readDB, err := openMetadataReadDatabase(ctx, cfg)
	if err != nil {
		t.Fatalf("open read pool: %v", err)
	}
	defer readDB.Close()

	const readers = 8
	var wait sync.WaitGroup
	errs := make(chan error, readers)
	start := make(chan struct{})
	for i := 0; i < readers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			var count int
			if err := readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&count); err != nil {
				errs <- err
			}
		}()
	}
	close(start)

	finished := make(chan struct{})
	go func() { wait.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent reads did not complete; the read pool is serialising")
	}
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent read failed: %v", err)
	}
}

// Reconciliation must not run on every claim, or a fleet of polling runners
// turns each poll into a full scan of the organisation's running targets.
func TestReconciliationIsThrottledPerOrganisation(t *testing.T) {
	throttle := newReconcileThrottle()
	now := time.Now()

	if !throttle.due("org_a", now) {
		t.Fatal("first reconciliation for an organisation should run")
	}
	if throttle.due("org_a", now.Add(leaseReconcileInterval/2)) {
		t.Fatal("a second reconciliation inside the interval should be skipped")
	}
	if !throttle.due("org_a", now.Add(leaseReconcileInterval+time.Second)) {
		t.Fatal("reconciliation should run again once the interval elapses")
	}
	// Throttling is per organisation, so one busy tenant cannot starve another.
	if !throttle.due("org_b", now) {
		t.Fatal("a different organisation should not be throttled by the first")
	}
}

// Concurrent claims for the same organisation must yield exactly one
// reconciliation per interval.
func TestReconcileThrottleAllowsOneWinnerUnderConcurrency(t *testing.T) {
	throttle := newReconcileThrottle()
	now := time.Now()

	const callers = 32
	var wait sync.WaitGroup
	results := make(chan bool, callers)
	for i := 0; i < callers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- throttle.due("org_demo", now)
		}()
	}
	wait.Wait()
	close(results)

	granted := 0
	for allowed := range results {
		if allowed {
			granted++
		}
	}
	if granted != 1 {
		t.Fatalf("%d concurrent callers reconciled, want exactly 1", granted)
	}
}

// Even when no runner is polling, abandoned work must still be dead-lettered.
// The background sweep is what guarantees that now reconciliation is throttled
// on the claim path.
func TestBackgroundSweepReconcilesWithoutAnyClaim(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	w := claimWithToken(t, mux, "rnr_local", "test-runner-token")
	if w.Code != 200 {
		t.Fatalf("claim failed: %d %s", w.Code, w.Body.String())
	}

	// Abandon the lease with no attempts left, then never poll again.
	if _, err := db.Exec(`UPDATE execution_targets
		SET attempt = max_attempts, lease_expires_at = datetime('now','-1 second')
		WHERE status = 'running'`); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	if err := reconcileAllOrganisationLeases(context.Background(), db); err != nil {
		t.Fatalf("background reconciliation failed: %v", err)
	}

	var targetStatus string
	if err := db.QueryRow(`SELECT status FROM execution_targets ORDER BY created_at DESC LIMIT 1`).Scan(&targetStatus); err != nil {
		t.Fatalf("read target status: %v", err)
	}
	if targetStatus != "dead_letter" {
		t.Fatalf("abandoned target status = %q, want dead_letter", targetStatus)
	}
}

// The background sweep must be safe to run when there is nothing to do.
func TestBackgroundSweepIsANoOpWhenNothingIsLeased(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	if err := reconcileAllOrganisationLeases(context.Background(), db); err != nil {
		t.Fatalf("reconciliation with no leased work failed: %v", err)
	}
}
