package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func migrate(ctx context.Context, db *sql.DB) error {
	if apiBackends.DatabaseDriver == "postgres" {
		return migratePostgres(ctx, db)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS organisations (
		id TEXT PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'active', settings TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL,
		user_type TEXT NOT NULL DEFAULT 'human', status TEXT NOT NULL DEFAULT 'active',
		external_subject TEXT, external_provider TEXT, last_login_at TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS memberships (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		user_id TEXT NOT NULL REFERENCES users(id), role TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, user_id)
	);
	CREATE TABLE IF NOT EXISTS servers (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		name TEXT NOT NULL, hostname TEXT, public_ip TEXT, private_ip TEXT,
		ssh_port INTEGER NOT NULL DEFAULT 22, ssh_username TEXT,
		environment TEXT NOT NULL DEFAULT 'development', provider TEXT,
		os_name TEXT, os_version TEXT, kernel_version TEXT, architecture TEXT,
		status TEXT NOT NULL DEFAULT 'pending',
		last_seen_at TEXT, last_check_at TEXT, archived_at TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, name)
	);
	CREATE TABLE IF NOT EXISTS server_tags (
		organisation_id TEXT NOT NULL REFERENCES organisations(id),
		server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
		key TEXT NOT NULL, value TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (organisation_id, server_id, key)
	);
	CREATE TABLE IF NOT EXISTS runners (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		name TEXT NOT NULL, runner_type TEXT NOT NULL DEFAULT 'customer_managed',
		status TEXT NOT NULL DEFAULT 'pending', version TEXT, hostname TEXT,
		platform TEXT, ip_address TEXT,
		last_seen_at TEXT, registered_at TEXT, revoked_at TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, name)
	);
	CREATE TABLE IF NOT EXISTS runner_scopes (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		runner_id TEXT NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
		scope_type TEXT NOT NULL, scope_value TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(runner_id, scope_type, scope_value)
	);
	CREATE TABLE IF NOT EXISTS runner_credentials (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		runner_id TEXT REFERENCES runners(id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE, expires_at TEXT NOT NULL,
		revoked_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS api_tokens (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		user_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
		token_prefix TEXT NOT NULL, token_hash TEXT NOT NULL UNIQUE,
		expires_at TEXT NOT NULL, revoked_at TEXT, last_used_at TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		actor_user_id TEXT NOT NULL REFERENCES users(id),
		actor_role_at_time TEXT NOT NULL,
		delegated_by_user_id TEXT REFERENCES users(id),
		approval_id TEXT,
		execution_type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'created',
		risk_level TEXT NOT NULL DEFAULT 'medium', environment TEXT, reason TEXT,
		command TEXT NOT NULL DEFAULT '', command_preview TEXT, command_hash TEXT,
		timeout_seconds INTEGER NOT NULL DEFAULT 300,
		requested_at TEXT NOT NULL DEFAULT (datetime('now')),
		queued_at TEXT, started_at TEXT, finished_at TEXT, error_summary TEXT,
		metadata TEXT NOT NULL DEFAULT '{}'
	);
	CREATE TABLE IF NOT EXISTS execution_targets (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
		server_id TEXT NOT NULL REFERENCES servers(id),
		runner_id TEXT REFERENCES runners(id),
		status TEXT NOT NULL DEFAULT 'pending',
		server_snapshot TEXT NOT NULL DEFAULT '{}',
		started_at TEXT, finished_at TEXT, exit_code INTEGER,
		stdout_bytes INTEGER NOT NULL DEFAULT 0, stderr_bytes INTEGER NOT NULL DEFAULT 0,
		stdout TEXT NOT NULL DEFAULT '', stderr TEXT NOT NULL DEFAULT '',
		stdout_artifact_id TEXT, stderr_artifact_id TEXT,
		lease_id TEXT, lease_expires_at TEXT, attempt INTEGER NOT NULL DEFAULT 0,
		max_attempts INTEGER NOT NULL DEFAULT 3, next_attempt_at TEXT,
		error_summary TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(execution_id, server_id)
	);
	CREATE TABLE IF NOT EXISTS execution_result_receipts (
		organisation_id TEXT NOT NULL REFERENCES organisations(id),
		execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
		target_id TEXT NOT NULL REFERENCES execution_targets(id) ON DELETE CASCADE,
		lease_id TEXT NOT NULL,
		runner_id TEXT NOT NULL,
		payload_hash TEXT NOT NULL,
		response_code INTEGER NOT NULL DEFAULT 200,
		response_body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (target_id, lease_id)
	);
	CREATE TABLE IF NOT EXISTS execution_idempotency (
		organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id),
		idempotency_key TEXT NOT NULL,
		payload_hash TEXT NOT NULL,
		execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
		response_body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (organisation_id, idempotency_key)
	);
	CREATE TABLE IF NOT EXISTS runbook_idempotency (
		organisation_id TEXT NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id),
		idempotency_key TEXT NOT NULL,
		payload_hash TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		response_status INTEGER NOT NULL,
		response_body TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		PRIMARY KEY (organisation_id, idempotency_key)
	);
	CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
		actor_type TEXT NOT NULL DEFAULT 'user', actor_user_id TEXT REFERENCES users(id),
		actor_display TEXT, actor_role_at_time TEXT,
		action TEXT NOT NULL, target_type TEXT, target_id TEXT, target_display TEXT,
		environment TEXT, result TEXT NOT NULL, reason TEXT,
		command_hash TEXT, command_preview TEXT,
		metadata TEXT NOT NULL DEFAULT '{}',
		previous_hash TEXT NOT NULL DEFAULT '',
		event_hash TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS runbooks (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		name TEXT NOT NULL, title TEXT NOT NULL, description TEXT,
		status TEXT NOT NULL DEFAULT 'draft', current_version_id TEXT,
		created_by_user_id TEXT NOT NULL REFERENCES users(id),
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, name)
	);
	CREATE TABLE IF NOT EXISTS runbook_versions (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		runbook_id TEXT NOT NULL REFERENCES runbooks(id) ON DELETE CASCADE,
		version INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'draft',
		risk_level TEXT NOT NULL DEFAULT 'medium',
		definition_yaml TEXT NOT NULL, definition_json TEXT NOT NULL,
		parameter_schema TEXT NOT NULL DEFAULT '{}',
		target_constraints TEXT NOT NULL DEFAULT '{}',
		approval_rules TEXT NOT NULL DEFAULT '{}',
		allowed_roles TEXT NOT NULL DEFAULT '["senior_engineer","admin","owner"]',
		command_preview TEXT, command_hash TEXT,
		created_by_user_id TEXT NOT NULL REFERENCES users(id),
		published_by_user_id TEXT REFERENCES users(id),
		published_at TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(runbook_id, version)
	);
	CREATE TABLE IF NOT EXISTS approval_requests (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		requester_user_id TEXT NOT NULL REFERENCES users(id),
		approver_user_id TEXT REFERENCES users(id),
		action_type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending',
		risk_level TEXT NOT NULL DEFAULT 'medium', reason TEXT NOT NULL,
		target_type TEXT NOT NULL, target_id TEXT,
		target_snapshot TEXT NOT NULL DEFAULT '{}',
		request_payload TEXT NOT NULL DEFAULT '{}',
		expires_at TEXT NOT NULL,
		decided_at TEXT, decision_note TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS execution_events (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		execution_id TEXT NOT NULL REFERENCES executions(id) ON DELETE CASCADE,
		target_id TEXT REFERENCES execution_targets(id) ON DELETE CASCADE,
		from_status TEXT, to_status TEXT NOT NULL, event_type TEXT NOT NULL,
		metadata TEXT NOT NULL DEFAULT '{}', occurred_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS artifact_records (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		owner_type TEXT NOT NULL, owner_id TEXT NOT NULL, content_type TEXT NOT NULL,
		byte_size INTEGER NOT NULL DEFAULT 0, sha256 TEXT NOT NULL, backend TEXT NOT NULL DEFAULT 'local',
		created_at TEXT NOT NULL DEFAULT (datetime('now')), deleted_at TEXT
	);
	CREATE TABLE IF NOT EXISTS automation_schedules (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		created_by_user_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
		runbook_name TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL,
		params TEXT NOT NULL DEFAULT '{}', interval_seconds INTEGER NOT NULL,
		next_run_at TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
		last_run_at TEXT, last_error TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, name)
	);
	CREATE TABLE IF NOT EXISTS automation_controls (
		organisation_id TEXT PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
		paused INTEGER NOT NULL DEFAULT 0,
		paused_at TEXT,
		paused_by_user_id TEXT REFERENCES users(id),
		reason TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS ai_requests (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		actor_user_id TEXT NOT NULL REFERENCES users(id), status TEXT NOT NULL,
		request_json TEXT NOT NULL DEFAULT '{}', response_text TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '', provider_request_id TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0, error_summary TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS ai_evidence (
		id TEXT PRIMARY KEY, request_id TEXT NOT NULL REFERENCES ai_requests(id) ON DELETE CASCADE,
		organisation_id TEXT NOT NULL REFERENCES organisations(id), ordinal INTEGER NOT NULL,
		kind TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL, source_uri TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_servers_org_status ON servers(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_servers_org_env ON servers(organisation_id, environment);
	CREATE INDEX IF NOT EXISTS idx_server_tags_org_kv ON server_tags(organisation_id, key, value);
	CREATE INDEX IF NOT EXISTS idx_server_tags_server ON server_tags(server_id);
	CREATE INDEX IF NOT EXISTS idx_runners_org_status ON runners(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_runners_last_seen ON runners(organisation_id, last_seen_at);
	CREATE INDEX IF NOT EXISTS idx_runner_scopes_runner ON runner_scopes(runner_id);
	CREATE INDEX IF NOT EXISTS idx_runner_credentials_hash ON runner_credentials(token_hash);
	CREATE INDEX IF NOT EXISTS idx_runner_credentials_runner ON runner_credentials(runner_id);
	CREATE INDEX IF NOT EXISTS idx_api_tokens_hash ON api_tokens(token_hash);
	CREATE INDEX IF NOT EXISTS idx_api_tokens_org_user ON api_tokens(organisation_id, user_id);
	CREATE INDEX IF NOT EXISTS idx_executions_org_status ON executions(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_executions_actor ON executions(actor_user_id, requested_at);
	CREATE INDEX IF NOT EXISTS idx_execution_targets_exec ON execution_targets(execution_id);
	CREATE INDEX IF NOT EXISTS idx_execution_targets_server ON execution_targets(server_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_result_receipts_execution ON execution_result_receipts(organisation_id, execution_id);
	CREATE INDEX IF NOT EXISTS idx_execution_idempotency_execution ON execution_idempotency(organisation_id, execution_id);
	CREATE INDEX IF NOT EXISTS idx_runbook_idempotency_resource ON runbook_idempotency(organisation_id, resource_type, resource_id);
	CREATE INDEX IF NOT EXISTS idx_audit_events_org_time ON audit_events(organisation_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(organisation_id, actor_user_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(organisation_id, action, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_target ON audit_events(organisation_id, target_type, target_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_runbooks_org_status ON runbooks(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_runbook_versions_runbook ON runbook_versions(runbook_id, version);
	CREATE INDEX IF NOT EXISTS idx_approvals_org_status ON approval_requests(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_approvals_requester ON approval_requests(requester_user_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_execution_events_execution ON execution_events(execution_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_artifact_records_owner ON artifact_records(organisation_id, owner_type, owner_id);
	CREATE INDEX IF NOT EXISTS idx_automation_schedules_due ON automation_schedules(organisation_id, enabled, next_run_at);
	CREATE INDEX IF NOT EXISTS idx_ai_requests_org_time ON ai_requests(organisation_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_ai_evidence_request ON ai_evidence(request_id, ordinal);
	`
	_, err := db.ExecContext(ctx, schema)
	if err != nil {
		return err
	}
	// Keep newly introduced tables in their own statement as well. This makes
	// upgrades reliable with SQLite drivers that do not execute every statement
	// in a multi-statement schema string.
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS automation_schedules (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		created_by_user_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
		runbook_name TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL,
		params TEXT NOT NULL DEFAULT '{}', interval_seconds INTEGER NOT NULL,
		next_run_at TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
		last_run_at TEXT, last_error TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')), UNIQUE(organisation_id, name)
	);`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_automation_schedules_due ON automation_schedules(organisation_id, enabled, next_run_at)`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS automation_controls (
		organisation_id TEXT PRIMARY KEY REFERENCES organisations(id) ON DELETE CASCADE,
		paused INTEGER NOT NULL DEFAULT 0, paused_at TEXT,
		paused_by_user_id TEXT REFERENCES users(id), reason TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);`); err != nil {
		return err
	}
	// Idempotent migrations for existing databases
	_ = addColumnIgnoreErr(ctx, db, "executions", "delegated_by_user_id", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "executions", "approval_id", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "executions", "command", "TEXT NOT NULL DEFAULT ''")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "stdout", "TEXT NOT NULL DEFAULT ''")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "stderr", "TEXT NOT NULL DEFAULT ''")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "stdout_artifact_id", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "stderr_artifact_id", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "lease_id", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "lease_expires_at", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "attempt", "INTEGER NOT NULL DEFAULT 0")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "max_attempts", "INTEGER NOT NULL DEFAULT 3")
	_ = addColumnIgnoreErr(ctx, db, "execution_targets", "next_attempt_at", "TEXT")
	_ = addColumnIgnoreErr(ctx, db, "runbook_versions", "allowed_roles", "TEXT NOT NULL DEFAULT '[\"senior_engineer\",\"admin\",\"owner\"]'")
	_ = addColumnIgnoreErr(ctx, db, "runner_credentials", "runner_id", "TEXT REFERENCES runners(id) ON DELETE CASCADE")
	_ = addColumnIgnoreErr(ctx, db, "audit_events", "previous_hash", "TEXT NOT NULL DEFAULT ''")
	_ = addColumnIgnoreErr(ctx, db, "audit_events", "event_hash", "TEXT NOT NULL DEFAULT ''")
	if err := backfillAuditHashChain(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_runner_credentials_runner ON runner_credentials(runner_id)`); err != nil {
		return err
	}
	return nil
}

func addColumnIgnoreErr(ctx context.Context, db *sql.DB, table, column, def string) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def))
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

func seed(ctx context.Context, db *sql.DB) error {
	if apiBackends.DatabaseDriver == "postgres" {
		return seedPostgres(ctx, db)
	}

	stmt := `
	INSERT OR IGNORE INTO organisations (id, name, slug) VALUES ('org_demo', 'Demo Org', 'demo');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_auditor', 'auditor@demo.local', 'Auditor');
	INSERT OR IGNORE INTO users (id, email, display_name, user_type) VALUES ('user_automation', 'automation@system.local', 'VPS Tools Automation', 'service');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_auditor', 'org_demo', 'user_auditor', 'auditor');
	INSERT OR IGNORE INTO servers (id, organisation_id, name, hostname, environment, status) VALUES ('srv_demo', 'org_demo', 'demo', 'localhost', 'development', 'active');
	INSERT OR IGNORE INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'role', 'app');
	INSERT OR IGNORE INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'env', 'development');
	INSERT OR IGNORE INTO runners (id, organisation_id, name, runner_type, status, last_seen_at, registered_at) VALUES ('rnr_local', 'org_demo', 'local-runner', 'customer_managed', 'active', datetime('now'), datetime('now'));
	INSERT OR IGNORE INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES ('rsc_local', 'org_demo', 'rnr_local', 'all', '*');
	INSERT OR IGNORE INTO runbooks (id, organisation_id, name, title, description, status, current_version_id, created_by_user_id) VALUES ('rbk_demo', 'org_demo', 'check-uptime', 'Check Uptime', 'Check server uptime and load', 'published', 'rbv_demo_v1', 'user_senior');
	INSERT OR IGNORE INTO runbook_versions (id, organisation_id, runbook_id, version, status, risk_level, allowed_roles, definition_yaml, definition_json, command_preview, command_hash, target_constraints, created_by_user_id, published_by_user_id, published_at) VALUES ('rbv_demo_v1', 'org_demo', 'rbk_demo', 1, 'published', 'low', '["senior_engineer","junior_engineer","admin","owner"]', '{}', '{"apiVersion":"vps-tools.io/v1","kind":"Runbook","spec":{"execution":{"command":"uptime"}}}', 'uptime', '', '{"allowedEnvironments":["development","staging"]}', 'user_senior', 'user_senior', datetime('now'));
	`
	_, err := db.ExecContext(ctx, stmt)
	return err
}
