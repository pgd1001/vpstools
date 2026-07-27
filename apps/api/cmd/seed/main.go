package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/svrtools?sslmode=disable"
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.Ping(pingCtx); err != nil {
		fmt.Fprintf(os.Stderr, "unable to verify database connection: %v\n", err)
		os.Exit(1)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to begin seed transaction: %v\n", err)
		os.Exit(1)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
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

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer')
		ON CONFLICT (organisation_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_auditor', 'org_demo', 'user_auditor', 'auditor')
		ON CONFLICT (organisation_id, user_id) DO UPDATE SET role = EXCLUDED.role, status = 'active';

		INSERT INTO servers (id, organisation_id, name, hostname, environment, status)
		VALUES ('srv_demo', 'org_demo', 'demo', 'localhost', 'development', 'active')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO server_tags (organisation_id, server_id, key, value)
		VALUES ('org_demo', 'srv_demo', 'role', 'app')
		ON CONFLICT DO NOTHING;

		INSERT INTO server_tags (organisation_id, server_id, key, value)
		VALUES ('org_demo', 'srv_demo', 'env', 'development')
		ON CONFLICT DO NOTHING;

		INSERT INTO runners (id, organisation_id, name, runner_type, status, last_seen_at, registered_at)
		VALUES ('rnr_local', 'org_demo', 'local-runner', 'customer_managed', 'active', now(), now())
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value)
		VALUES ('rsc_local', 'org_demo', 'rnr_local', 'all', '*')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO runbooks (id, organisation_id, name, title, description, status, current_version_id, created_by_user_id)
		VALUES ('rbk_demo', 'org_demo', 'check-uptime', 'Check Uptime', 'Check server uptime and load', 'published', 'rbv_demo_v1', 'user_senior')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO runbook_versions (id, organisation_id, runbook_id, version, status, risk_level, allowed_roles, definition_yaml, definition_json, command_preview, command_hash, target_constraints, created_by_user_id, published_by_user_id, published_at)
		VALUES ('rbv_demo_v1', 'org_demo', 'rbk_demo', 1, 'published', 'low', '["senior_engineer","junior_engineer","admin","owner"]', '{}', '{"apiVersion":"vps-tools.io/v1","kind":"Runbook","spec":{"execution":{"command":"uptime"}}}', 'uptime', '', '{"allowedEnvironments":["development","staging"]}', 'user_senior', 'user_senior', now())
		ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}
	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "seed commit failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("seed completed")
}
