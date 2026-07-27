package main

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// postgresSchemaContract is the minimum catalog contract required by the API
// handlers. It is intentionally kept here, beside the runtime database
// boundary, so a future removal of the PostgreSQL startup guard has an
// explicit fail-closed check before serving requests.
var postgresSchemaContract = map[string][]string{
	"organisations":             {"id", "name", "slug", "settings"},
	"users":                     {"id", "email", "display_name", "status"},
	"memberships":               {"organisation_id", "user_id", "role", "status"},
	"servers":                   {"id", "organisation_id", "name", "status", "metadata"},
	"server_tags":               {"organisation_id", "server_id", "key", "value"},
	"runners":                   {"id", "organisation_id", "name", "status", "metadata"},
	"runner_scopes":             {"organisation_id", "runner_id", "scope_type", "scope_value"},
	"runner_credentials":        {"organisation_id", "runner_id", "token_hash", "expires_at", "revoked_at"},
	"api_tokens":                {"organisation_id", "user_id", "token_prefix", "token_hash", "expires_at", "revoked_at", "last_used_at"},
	"executions":                {"id", "organisation_id", "actor_user_id", "command", "status", "metadata"},
	"execution_targets":         {"organisation_id", "execution_id", "server_id", "status", "stdout", "stderr", "lease_id", "attempt", "max_attempts"},
	"execution_result_receipts": {"organisation_id", "execution_id", "target_id", "lease_id", "runner_id", "payload_hash", "response_body"},
	"execution_idempotency":     {"organisation_id", "user_id", "idempotency_key", "payload_hash", "execution_id", "response_body"},
	"runbook_idempotency":       {"organisation_id", "user_id", "idempotency_key", "resource_type", "resource_id", "response_status", "response_body"},
	"audit_events":              {"id", "organisation_id", "actor_user_id", "action", "result", "metadata", "previous_hash", "event_hash"},
	"runbooks":                  {"id", "organisation_id", "name", "status", "current_version_id"},
	"runbook_versions":          {"id", "organisation_id", "runbook_id", "version", "status", "definition_json", "allowed_roles"},
	"approval_requests":         {"id", "organisation_id", "requester_user_id", "status", "reason", "expires_at"},
	"execution_events":          {"id", "organisation_id", "execution_id", "to_status", "event_type", "metadata"},
	"artifact_records":          {"id", "organisation_id", "owner_type", "owner_id", "sha256", "backend"},
	"automation_schedules":      {"id", "organisation_id", "created_by_user_id", "runbook_name", "next_run_at", "enabled"},
	"automation_controls":       {"organisation_id", "paused", "updated_at"},
	"ai_requests":               {"id", "organisation_id", "actor_user_id", "status", "request_json", "response_text"},
	"ai_evidence":               {"id", "request_id", "organisation_id", "ordinal", "content"},
}

type postgresSchemaColumn struct {
	Table  string
	Column string
}

// verifyPostgresSchema checks the live database catalog without mutating it.
// It must run after Goose migrations and before handlers are registered when
// PostgreSQL support is enabled.
func verifyPostgresSchema(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = ANY($1)
	`, postgresContractTables())
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL schema: %w", err)
	}
	defer rows.Close()

	var columns []postgresSchemaColumn
	for rows.Next() {
		var column postgresSchemaColumn
		if err := rows.Scan(&column.Table, &column.Column); err != nil {
			return fmt.Errorf("read PostgreSQL schema: %w", err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read PostgreSQL schema: %w", err)
	}
	return validatePostgresSchemaColumns(columns)
}

func postgresContractTables() []string {
	tables := make([]string, 0, len(postgresSchemaContract))
	for table := range postgresSchemaContract {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

func validatePostgresSchemaColumns(columns []postgresSchemaColumn) error {
	present := make(map[string]map[string]bool, len(columns))
	for _, column := range columns {
		if present[column.Table] == nil {
			present[column.Table] = make(map[string]bool)
		}
		present[column.Table][column.Column] = true
	}

	var missing []string
	for _, table := range postgresContractTables() {
		for _, column := range postgresSchemaContract[table] {
			if !present[table][column] {
				missing = append(missing, table+"."+column)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("PostgreSQL schema is missing required columns: %s", strings.Join(missing, ", "))
	}
	return nil
}
