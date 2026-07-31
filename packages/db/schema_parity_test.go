package db

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The hand-maintained storage-contract check that lived here only verified
// columns someone remembered to add to a list, so drift in any unlisted
// column passed silently. TestSchemaParityIsComplete in schema_drift_test.go
// now derives both schemas and compares them in full.

func TestPostgresMigrationsAreOrderedAndComplete(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatal("no PostgreSQL migrations found")
	}

	for index, path := range paths {
		name := filepath.Base(path)
		prefix := strings.SplitN(name, "_", 2)[0]
		version, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatalf("migration %s has invalid numeric prefix: %v", name, err)
		}
		if version != index+1 {
			t.Fatalf("migration %s has version %d, want %d", name, version, index+1)
		}
		contents := string(readTestFile(t, path))
		if !strings.Contains(contents, "-- +goose Up") {
			t.Errorf("migration %s has no Goose Up section", name)
		}
		if !strings.Contains(contents, "-- +goose Down") {
			t.Errorf("migration %s has no Goose Down section", name)
		}
	}
}

func TestPostgresMigrationsContainAllRuntimeTables(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var schema strings.Builder
	for _, path := range paths {
		schema.Write(extractGooseUp(readTestFile(t, path)))
		schema.WriteByte('\n')
	}

	for _, table := range []string{
		"organisations", "users", "memberships", "servers", "server_tags", "runners", "runner_scopes",
		"runner_credentials", "api_tokens", "executions", "execution_targets", "execution_result_receipts",
		"execution_idempotency", "runbook_idempotency", "audit_events", "runbooks", "runbook_versions",
		"approval_requests", "execution_events", "artifact_records", "automation_schedules", "automation_controls",
		"ai_requests", "ai_evidence",
	} {
		pattern := regexp.MustCompile(`(?i)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+` + regexp.QuoteMeta(table) + `(?:\s|\()`)
		if !pattern.MatchString(schema.String()) {
			t.Errorf("PostgreSQL migrations do not create runtime table %s", table)
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
