package db

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestSQLiteAndPostgresStorageContract is deliberately text based. It checks
// the shared storage contract without requiring PostgreSQL, Docker, or a
// network. It does not claim that the API handlers support PostgreSQL. The
// startup guard remains the authority for that runtime decision.
func TestSQLiteAndPostgresStorageContract(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	runtimeSQL := readTestFile(t, filepath.Join(root, "apps", "api", "migrate.go"))

	paths, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var postgresSQL strings.Builder
	for _, path := range paths {
		postgresSQL.Write(extractGooseUp(readTestFile(t, path)))
		postgresSQL.WriteByte('\n')
	}

	// These are fields deliberately shared by the SQLite runtime and the
	// PostgreSQL schema. This is a storage check, not a PostgreSQL support list.
	contract := map[string][]string{
		"runner_credentials":        {"runner_id", "token_hash", "expires_at", "revoked_at"},
		"api_tokens":                {"user_id", "token_prefix", "token_hash", "expires_at", "revoked_at", "last_used_at"},
		"executions":                {"command", "actor_user_id", "requested_at", "metadata"},
		"execution_result_receipts": {"payload_hash", "response_code", "response_body", "lease_id", "runner_id"},
		"execution_idempotency":     {"user_id", "idempotency_key", "payload_hash", "response_body"},
		"runbook_idempotency":       {"user_id", "idempotency_key", "payload_hash", "response_status", "response_body"},
		"automation_schedules":      {"created_by_user_id", "runbook_name", "params", "interval_seconds", "next_run_at", "enabled", "last_run_at", "last_error"},
		"automation_controls":       {"paused", "paused_at", "paused_by_user_id", "reason", "updated_at"},
		"audit_events":              {"actor_user_id", "metadata", "previous_hash", "event_hash", "occurred_at"},
		"ai_requests":               {"actor_user_id", "status", "request_json", "response_text", "model", "provider_request_id", "duration_ms", "error_summary", "created_at"},
		"ai_evidence":               {"request_id", "organisation_id", "ordinal", "kind", "title", "content", "source_uri", "created_at"},
	}
	for table, columns := range contract {
		for _, column := range columns {
			if !hasColumn(runtimeSQL, table, column) {
				t.Errorf("SQLite runtime is missing %s.%s", table, column)
			}
			if !hasColumn([]byte(postgresSQL.String()), table, column) {
				t.Errorf("PostgreSQL migrations are missing %s.%s", table, column)
			}
		}
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func hasColumn(schema []byte, table, column string) bool {
	tableExpr := regexp.QuoteMeta(table)
	columnExpr := `\b` + regexp.QuoteMeta(column) + `\b`
	create := regexp.MustCompile(`(?is)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+` + tableExpr + `\s*\((.*?)\);`)
	for _, match := range create.FindAllSubmatch(schema, -1) {
		if regexp.MustCompile(`(?i)` + columnExpr).Match(match[1]) {
			return true
		}
	}
	// Upgrade migrations may add a field without repeating the table body.
	alter := regexp.MustCompile(`(?is)ALTER\s+TABLE\s+` + tableExpr + `\s+ADD\s+COLUMN(?:\s+IF\s+NOT\s+EXISTS)?\s+` + columnExpr + `[^;]*;`)
	if alter.Match(schema) {
		return true
	}
	// PostgreSQL permits one ALTER TABLE statement to add several columns.
	// In that form the requested column may occur after the first ADD COLUMN.
	batchedAlter := regexp.MustCompile(`(?is)ALTER\s+TABLE\s+` + tableExpr + `[^;]*\bADD\s+COLUMN[^;]*\b` + columnExpr + `\b[^;]*;`)
	return batchedAlter.Match(schema)
}

func extractGooseUp(schema []byte) []byte {
	var up strings.Builder
	for _, section := range strings.Split(string(schema), "-- +goose ") {
		if strings.HasPrefix(section, "Up") {
			up.WriteString(strings.TrimPrefix(section, "Up"))
		}
	}
	return []byte(up.String())
}
