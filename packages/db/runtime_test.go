package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

type recordingExecer struct {
	query    string
	affected int64
}

func (r *recordingExecer) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	r.query = query
	return driverResult(r.affected), nil
}

type driverResult int64

func (driverResult) LastInsertId() (int64, error)   { return 0, nil }
func (r driverResult) RowsAffected() (int64, error) { return int64(r), nil }

func TestRuntimeRebindsPostgreSQLAndPreservesSQLite(t *testing.T) {
	postgres, err := NewRuntime("postgres")
	if err != nil {
		t.Fatal(err)
	}
	sqlite, err := NewRuntime("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	query := "UPDATE jobs SET status = ? WHERE id = ? AND note = '?'"
	if got := postgres.Rebind(query); got != "UPDATE jobs SET status = $1 WHERE id = $2 AND note = '?'" {
		t.Fatalf("PostgreSQL query = %q", got)
	}
	if got := sqlite.Rebind(query); got != query {
		t.Fatalf("SQLite query = %q, want original query", got)
	}
	recorder := &recordingExecer{}
	if _, err := postgres.ExecContext(context.Background(), recorder, query, "running", "job-1"); err != nil {
		t.Fatal(err)
	}
	if recorder.query != "UPDATE jobs SET status = $1 WHERE id = $2 AND note = '?'" {
		t.Fatalf("executed query = %q", recorder.query)
	}
}

func TestRuntimeJSONParameter(t *testing.T) {
	postgres, _ := NewRuntime("postgres")
	sqlite, _ := NewRuntime("sqlite")
	if postgres.JSONParameter() != "?::jsonb" || sqlite.JSONParameter() != "?" {
		t.Fatalf("unexpected JSON parameter expressions")
	}
}

func TestCheckedExecContextRequiresExactlyOneRow(t *testing.T) {
	runtime, err := NewRuntime("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []bool{true, false} {
		recorder := &recordingExecer{}
		if want {
			recorder.affected = 1
		} else {
			recorder.affected = 0
		}
		got, err := runtime.CheckedExecContext(context.Background(), recorder, "UPDATE leases SET active = ? WHERE id = ?", true, "lease-1")
		if err != nil || got != want {
			t.Fatalf("CheckedExecContext() = %v, %v, want %v", got, err, want)
		}
	}
}

func TestConcurrentCheckedUpdatesClaimOneSQLiteTarget(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "queue.db")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	if _, err := db.Exec("CREATE TABLE execution_targets (id TEXT PRIMARY KEY, status TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO execution_targets (id, status) VALUES ('target-1', 'pending')"); err != nil {
		t.Fatal(err)
	}
	runtime, _ := NewRuntime("sqlite")
	results := make(chan bool, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			claimed, err := runtime.CheckedExecContext(context.Background(), db,
				"UPDATE execution_targets SET status = 'running' WHERE id = ? AND status = 'pending'", "target-1")
			results <- err == nil && claimed
		}()
	}
	group.Wait()
	close(results)
	claimed := 0
	for result := range results {
		if result {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("concurrent claims succeeded %d times, want exactly once", claimed)
	}
}
