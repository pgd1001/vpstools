package main

import (
	"context"
	"database/sql"
)

func migrate(ctx context.Context, db *sql.DB) error {
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
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		actor_user_id TEXT NOT NULL REFERENCES users(id),
		actor_role_at_time TEXT NOT NULL,
		execution_type TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'created',
		risk_level TEXT NOT NULL DEFAULT 'medium', environment TEXT, reason TEXT,
		command_preview TEXT, command_hash TEXT,
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
		error_summary TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(execution_id, server_id)
	);
	CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		occurred_at TEXT NOT NULL DEFAULT (datetime('now')),
		actor_type TEXT NOT NULL DEFAULT 'user', actor_user_id TEXT REFERENCES users(id),
		actor_display TEXT, actor_role_at_time TEXT,
		action TEXT NOT NULL, target_type TEXT, target_id TEXT, target_display TEXT,
		environment TEXT, result TEXT NOT NULL, reason TEXT,
		command_hash TEXT, command_preview TEXT,
		metadata TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX IF NOT EXISTS idx_servers_org_status ON servers(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_servers_org_env ON servers(organisation_id, environment);
	CREATE INDEX IF NOT EXISTS idx_server_tags_org_kv ON server_tags(organisation_id, key, value);
	CREATE INDEX IF NOT EXISTS idx_server_tags_server ON server_tags(server_id);
	CREATE INDEX IF NOT EXISTS idx_runners_org_status ON runners(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_runners_last_seen ON runners(organisation_id, last_seen_at);
	CREATE INDEX IF NOT EXISTS idx_runner_scopes_runner ON runner_scopes(runner_id);
	CREATE INDEX IF NOT EXISTS idx_executions_org_status ON executions(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_executions_actor ON executions(actor_user_id, requested_at);
	CREATE INDEX IF NOT EXISTS idx_execution_targets_exec ON execution_targets(execution_id);
	CREATE INDEX IF NOT EXISTS idx_execution_targets_server ON execution_targets(server_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_org_time ON audit_events(organisation_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(organisation_id, actor_user_id, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(organisation_id, action, occurred_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_target ON audit_events(organisation_id, target_type, target_id, occurred_at);
	`
	_, err := db.ExecContext(ctx, schema)
	return err
}

func seed(ctx context.Context, db *sql.DB) error {
	stmt := `
	INSERT OR IGNORE INTO organisations (id, name, slug) VALUES ('org_demo', 'Demo Org', 'demo');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer');
	INSERT OR IGNORE INTO users (id, email, display_name) VALUES ('user_auditor', 'auditor@demo.local', 'Auditor');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_auditor', 'org_demo', 'user_auditor', 'auditor');
	INSERT OR IGNORE INTO servers (id, organisation_id, name, hostname, environment, status) VALUES ('srv_demo', 'org_demo', 'demo-server', 'localhost', 'development', 'active');
	INSERT OR IGNORE INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'role', 'app');
	INSERT OR IGNORE INTO server_tags (organisation_id, server_id, key, value) VALUES ('org_demo', 'srv_demo', 'env', 'development');
	INSERT OR IGNORE INTO runners (id, organisation_id, name, runner_type, status, last_seen_at, registered_at) VALUES ('rnr_local', 'org_demo', 'local-runner', 'customer_managed', 'active', datetime('now'), datetime('now'));
	INSERT OR IGNORE INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES ('rsc_local', 'org_demo', 'rnr_local', 'all', '*');
	`
	_, err := db.ExecContext(ctx, stmt)
	return err
}
