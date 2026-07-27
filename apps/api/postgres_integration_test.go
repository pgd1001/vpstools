package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// TestPostgresRuntimeIntegration is opt-in because the default development
// environment is deliberately self-contained. CI or a release rehearsal can
// set POSTGRES_TEST_URL to run the real migration and catalog contract against
// a disposable PostgreSQL instance.
func TestPostgresRuntimeIntegration(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	previousBackends := apiBackends
	apiBackends.DatabaseDriver = "postgres"
	defer func() { apiBackends = previousBackends }()
	if err := migratePostgres(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := seedPostgres(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM organisations WHERE id = 'org_demo'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("seeded organisation count = %d, want 1", count)
	}
	apiBackends.PostgresRLS = true
	if err := configurePostgresRLS(ctx, db); err != nil {
		t.Fatal(err)
	}
	var policyCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE schemaname = current_schema() AND policyname LIKE 'vps_tenant_%'`).Scan(&policyCount); err != nil {
		t.Fatal(err)
	}
	if policyCount != 19 {
		t.Fatalf("tenant RLS policy count = %d, want 19", policyCount)
	}
	if _, err := db.ExecContext(ctx, `SELECT set_config('vps.organisation_id', 'org_demo', false)`); err != nil {
		t.Fatal(err)
	}
	var currentOrganisation string
	if err := db.QueryRowContext(ctx, `SELECT vps_current_organisation()`).Scan(&currentOrganisation); err != nil {
		t.Fatal(err)
	}
	if currentOrganisation != "org_demo" {
		t.Fatalf("current organisation = %q, want org_demo", currentOrganisation)
	}
}

func postgresIntegrationMigrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test source")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations", "postgres")
}
