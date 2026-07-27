package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pressly/goose/v3"
)

// migratePostgres applies the checked-in PostgreSQL migrations before the API
// starts serving requests. The self-contained SQLite path keeps its existing
// embedded schema, while PostgreSQL uses the versioned migration history that
// is also used by the migration command and CI integration test.
func migratePostgres(ctx context.Context, db *sql.DB) error {
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure PostgreSQL migration dialect: %w", err)
	}
	dir, err := postgresMigrationDir()
	if err != nil {
		return err
	}
	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("apply PostgreSQL migrations: %w", err)
	}
	if err := verifyPostgresSchema(ctx, db); err != nil {
		return fmt.Errorf("verify PostgreSQL schema: %w", err)
	}
	return nil
}

func postgresMigrationDir() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to locate PostgreSQL migration source")
	}
	starts := []string{filepath.Dir(source), workingDirectoryForMigrations()}
	if executable, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(executable))
	}
	for _, start := range starts {
		if root, ok := migrationProjectRoot(start); ok {
			return filepath.Join(root, "migrations", "postgres"), nil
		}
	}
	return "", fmt.Errorf("unable to locate migrations/postgres beside the API source, working directory, or release archive")
}

func workingDirectoryForMigrations() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return ""
	}
	return workingDirectory
}

func migrationProjectRoot(start string) (string, bool) {
	path, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		candidate := filepath.Join(path, "migrations", "postgres")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return path, true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", false
		}
		path = parent
	}
}

func seedPostgres(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO organisations (id, name, slug) VALUES ('org_demo', 'Demo Org', 'demo')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO users (id, email, display_name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO users (id, email, display_name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO users (id, email, display_name) VALUES ('user_auditor', 'auditor@demo.local', 'Auditor')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO users (id, email, display_name, user_type) VALUES ('user_automation', 'automation@system.local', 'VPS Tools Automation', 'service')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer')
		ON CONFLICT (organisation_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';
		INSERT INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer')
		ON CONFLICT (organisation_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';
		INSERT INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_auditor', 'org_demo', 'user_auditor', 'auditor')
		ON CONFLICT (organisation_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';
		INSERT INTO servers (id, organisation_id, name, hostname, environment, status) VALUES ('srv_demo', 'org_demo', 'demo', 'localhost', 'development', 'active')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'role', 'app')
		ON CONFLICT DO NOTHING;
		INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'env', 'development')
		ON CONFLICT DO NOTHING;
		INSERT INTO runners (id, organisation_id, name, runner_type, status, last_seen_at, registered_at) VALUES ('rnr_local', 'org_demo', 'local-runner', 'customer_managed', 'active', now(), now())
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES ('rsc_local', 'org_demo', 'rnr_local', 'all', '*')
		ON CONFLICT DO NOTHING;
		INSERT INTO runbooks (id, organisation_id, name, title, description, status, current_version_id, created_by_user_id) VALUES ('rbk_demo', 'org_demo', 'check-uptime', 'Check Uptime', 'Check server uptime and load', 'published', 'rbv_demo_v1', 'user_senior')
		ON CONFLICT (id) DO NOTHING;
		INSERT INTO runbook_versions (id, organisation_id, runbook_id, version, status, risk_level, allowed_roles, definition_yaml, definition_json, command_preview, command_hash, target_constraints, created_by_user_id, published_by_user_id, published_at)
		VALUES ('rbv_demo_v1', 'org_demo', 'rbk_demo', 1, 'published', 'low', '["senior_engineer","junior_engineer","admin","owner"]', '{}', '{"apiVersion":"vps-tools.io/v1","kind":"Runbook","spec":{"execution":{"command":"uptime"}}}', 'uptime', '', '{"allowedEnvironments":["development","staging"]}', 'user_senior', 'user_senior', now())
		ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("seed PostgreSQL demo records: %w", err)
	}
	return nil
}

// configurePostgresRLS enables the optional defence-in-depth tenant boundary.
// Authentication tables stay outside this policy because token and OIDC
// resolution must happen before an organisation is known. Business tables
// still retain explicit organisation predicates in application queries.
func configurePostgresRLS(ctx context.Context, db *sql.DB) error {
	if apiBackends.DatabaseDriver != "postgres" || !apiBackends.PostgresRLS {
		return nil
	}
	var ownsTables bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_class c
		JOIN pg_roles r ON r.oid = c.relowner
		WHERE c.relname = 'servers' AND c.relnamespace = current_schema()::regnamespace
		  AND r.rolname = current_user
	)`).Scan(&ownsTables); err != nil {
		return fmt.Errorf("check PostgreSQL RLS table ownership: %w", err)
	}
	if !ownsTables {
		return verifyPostgresRLSPolicies(ctx, db)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE OR REPLACE FUNCTION vps_current_organisation() RETURNS text
		LANGUAGE sql STABLE AS $$ SELECT NULLIF(current_setting('vps.organisation_id', true), '') $$;
	`); err != nil {
		return fmt.Errorf("create PostgreSQL tenant function: %w", err)
	}
	tables := []string{
		"servers", "server_tags", "runners", "runner_scopes", "executions", "execution_targets",
		"execution_result_receipts", "execution_idempotency", "runbook_idempotency", "audit_events",
		"runbooks", "runbook_versions", "approval_requests", "execution_events", "artifact_records",
		"automation_schedules", "automation_controls", "ai_requests", "ai_evidence",
	}
	for _, table := range tables {
		policy := "vps_tenant_" + table
		statement := fmt.Sprintf(`
			ALTER TABLE %s ENABLE ROW LEVEL SECURITY;
			DROP POLICY IF EXISTS %s ON %s;
			CREATE POLICY %s ON %s
				USING (organisation_id = vps_current_organisation())
				WITH CHECK (organisation_id = vps_current_organisation());
		`, table, policy, table, policy, table)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure PostgreSQL RLS for %s: %w", table, err)
		}
	}
	return nil
}

func verifyPostgresRLSPolicies(ctx context.Context, db *sql.DB) error {
	var policyCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE schemaname = current_schema() AND policyname LIKE 'vps_tenant_%'`).Scan(&policyCount); err != nil {
		return fmt.Errorf("inspect PostgreSQL RLS policies: %w", err)
	}
	if policyCount != 19 {
		return fmt.Errorf("PostgreSQL RLS requires policies installed by the migration owner, found %d of 19", policyCount)
	}
	return nil
}
