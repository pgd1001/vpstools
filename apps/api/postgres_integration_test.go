package main

import (
	"context"
	"database/sql"
	"net/url"
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
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := sql.Open("pgx", databaseURL)
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
	tenantCtx, cleanup, err := bindTenantConnection(ctx, db, "org_demo")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if err := apiQueryRow(tenantCtx, db, `SELECT vps_current_organisation()`).Scan(&currentOrganisation); err != nil {
		t.Fatal(err)
	}
	if currentOrganisation != "org_demo" {
		t.Fatalf("bound tenant organisation = %q, want org_demo", currentOrganisation)
	}
	const rlsRole = "svrtools_rls_test"
	const rlsPassword = "svrtools-rls-test-password"
	if _, err := db.ExecContext(ctx, `DROP ROLE IF EXISTS svrtools_rls_test`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE ROLE svrtools_rls_test LOGIN PASSWORD 'svrtools-rls-test-password'`); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), `DROP ROLE IF EXISTS svrtools_rls_test`)
	if _, err := db.ExecContext(ctx, `GRANT USAGE ON SCHEMA public TO svrtools_rls_test; GRANT SELECT ON servers TO svrtools_rls_test;`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO organisations (id, name, slug) VALUES ('org_other', 'Other Org', 'other') ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO servers (id, organisation_id, name, hostname, environment, status) VALUES ('srv_other', 'org_other', 'other', 'other.local', 'development', 'active') ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(rlsRole, rlsPassword)
	rlsDB, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rlsDB.Close()
	if err := rlsDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := rlsDB.ExecContext(ctx, `SELECT set_config('vps.organisation_id', 'org_demo', false)`); err != nil {
		t.Fatal(err)
	}
	if err := rlsDB.QueryRowContext(ctx, `SELECT count(*) FROM servers`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RLS org_demo server count = %d, want 1", count)
	}
	if _, err := rlsDB.ExecContext(ctx, `SELECT set_config('vps.organisation_id', 'org_other', false)`); err != nil {
		t.Fatal(err)
	}
	if err := rlsDB.QueryRowContext(ctx, `SELECT count(*) FROM servers`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("RLS org_other server count = %d, want 1", count)
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
