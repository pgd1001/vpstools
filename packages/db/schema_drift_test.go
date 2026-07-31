package db

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// columnsByTable extracts a table -> column-set map from a SQL schema. It
// understands CREATE TABLE bodies and later ALTER TABLE ... ADD COLUMN
// statements, which is how both dialects accumulate their schema.
func columnsByTable(schema string) map[string]map[string]bool {
	tables := map[string]map[string]bool{}

	create := regexp.MustCompile(`(?is)CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_]+)\s*\((.*?)\);`)
	for _, match := range create.FindAllStringSubmatch(schema, -1) {
		table := strings.ToLower(match[1])
		if tables[table] == nil {
			tables[table] = map[string]bool{}
		}
		for _, column := range parseColumnNames(match[2]) {
			tables[table][column] = true
		}
	}

	// PostgreSQL allows one ALTER TABLE to add several columns in a comma
	// separated list, so match the whole statement and then every ADD COLUMN
	// within it rather than only the first.
	alterStatement := regexp.MustCompile(`(?is)ALTER\s+TABLE\s+([a-z_]+)\s+((?:[^;])*?);`)
	addColumn := regexp.MustCompile(`(?is)ADD\s+COLUMN(?:\s+IF\s+NOT\s+EXISTS)?\s+([a-z_]+)`)
	for _, match := range alterStatement.FindAllStringSubmatch(schema, -1) {
		table := strings.ToLower(match[1])
		added := addColumn.FindAllStringSubmatch(match[2], -1)
		if len(added) == 0 {
			continue
		}
		if tables[table] == nil {
			tables[table] = map[string]bool{}
		}
		for _, column := range added {
			tables[table][strings.ToLower(column[1])] = true
		}
	}

	return tables
}

// parseColumnNames pulls column names out of a CREATE TABLE body, skipping
// table-level constraints, which are not columns.
func parseColumnNames(body string) []string {
	var columns []string
	depth := 0
	current := strings.Builder{}
	flush := func() {
		definition := strings.TrimSpace(current.String())
		current.Reset()
		if definition == "" {
			return
		}
		fields := strings.Fields(definition)
		if len(fields) == 0 {
			return
		}
		name := strings.ToLower(strings.Trim(fields[0], `"`))
		switch name {
		case "primary", "unique", "foreign", "check", "constraint", "exclude":
			return
		}
		if !regexp.MustCompile(`^[a-z_][a-z0-9_]*$`).MatchString(name) {
			return
		}
		columns = append(columns, name)
	}
	for _, r := range body {
		switch r {
		case '(':
			depth++
			current.WriteRune(r)
		case ')':
			depth--
			current.WriteRune(r)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return columns
}

func schemaRoot(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(here), "..", ".."))
}

func sqliteRuntimeSchema(t *testing.T) string {
	t.Helper()
	return string(readTestFile(t, filepath.Join(schemaRoot(t), "apps", "api", "migrate.go")))
}

func postgresMigrationSchema(t *testing.T) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(schemaRoot(t), "migrations", "postgres", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var schema strings.Builder
	for _, path := range paths {
		schema.Write(extractGooseUp(readTestFile(t, path)))
		schema.WriteByte('\n')
	}
	return schema.String()
}

// TestColumnExtractionUnderstandsBothDialects pins the behaviour the parity
// check depends on. Without this, a parser bug would silently turn the parity
// test into one that always passes.
func TestColumnExtractionUnderstandsBothDialects(t *testing.T) {
	schema := `
	CREATE TABLE IF NOT EXISTS widgets (
		id TEXT PRIMARY KEY,
		organisation_id TEXT NOT NULL REFERENCES organisations(id),
		label TEXT NOT NULL DEFAULT 'x,y',
		size INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(organisation_id, label),
		PRIMARY KEY (id, organisation_id)
	);
	ALTER TABLE widgets ADD COLUMN colour TEXT;
	ALTER TABLE widgets
		ADD COLUMN IF NOT EXISTS weight INTEGER NOT NULL DEFAULT 0,
		ADD COLUMN IF NOT EXISTS height INTEGER NOT NULL DEFAULT 0;
	`
	tables := columnsByTable(schema)
	widgets, present := tables["widgets"]
	if !present {
		t.Fatal("widgets table was not parsed")
	}
	for _, column := range []string{"id", "organisation_id", "label", "size", "created_at", "colour", "weight", "height"} {
		if !widgets[column] {
			t.Errorf("column %q was not extracted", column)
		}
	}
	// Table-level constraints are not columns and must not be reported as such.
	for _, notAColumn := range []string{"unique", "primary", "foreign", "check"} {
		if widgets[notAColumn] {
			t.Errorf("constraint keyword %q was treated as a column", notAColumn)
		}
	}
	// A default containing a comma must not split into a phantom column.
	if len(widgets) != 8 {
		t.Errorf("extracted %d columns, want 8: %v", len(widgets), widgets)
	}
}

// TestParityCheckDetectsDrift proves the comparison finds a missing column in
// either direction, so a green parity test means something.
func TestParityCheckDetectsDrift(t *testing.T) {
	base := `CREATE TABLE things (id TEXT PRIMARY KEY, name TEXT NOT NULL);`
	extended := `CREATE TABLE things (id TEXT PRIMARY KEY, name TEXT NOT NULL, extra TEXT);`

	if diff := schemaDifferences(columnsByTable(base), columnsByTable(base)); len(diff) != 0 {
		t.Fatalf("identical schemas reported drift: %v", diff)
	}
	if diff := schemaDifferences(columnsByTable(base), columnsByTable(extended)); len(diff) == 0 {
		t.Fatal("a column present only in the second schema was not detected")
	}
	if diff := schemaDifferences(columnsByTable(extended), columnsByTable(base)); len(diff) == 0 {
		t.Fatal("a column present only in the first schema was not detected")
	}
	missingTable := `CREATE TABLE other (id TEXT PRIMARY KEY);`
	if diff := schemaDifferences(columnsByTable(base), columnsByTable(missingTable)); len(diff) == 0 {
		t.Fatal("a table present in only one schema was not detected")
	}
}

// schemaDifferences reports every table or column that is not present in both
// schemas, in both directions.
func schemaDifferences(sqliteTables, postgresTables map[string]map[string]bool) []string {
	var problems []string
	for table, sqliteColumns := range sqliteTables {
		postgresColumns, present := postgresTables[table]
		if !present {
			problems = append(problems, fmt.Sprintf("table %s exists in SQLite but not in the PostgreSQL migrations", table))
			continue
		}
		for column := range sqliteColumns {
			if !postgresColumns[column] {
				problems = append(problems, fmt.Sprintf("%s.%s exists in SQLite but not in PostgreSQL", table, column))
			}
		}
		for column := range postgresColumns {
			if !sqliteColumns[column] {
				problems = append(problems, fmt.Sprintf("%s.%s exists in PostgreSQL but not in SQLite", table, column))
			}
		}
	}
	for table := range postgresTables {
		if _, present := sqliteTables[table]; !present {
			problems = append(problems, fmt.Sprintf("table %s exists in the PostgreSQL migrations but not in SQLite", table))
		}
	}
	sort.Strings(problems)
	return problems
}

// TestSchemaParityIsComplete compares every table and column across the two
// dialects rather than a hand-maintained contract list.
//
// The previous parity check only verified columns someone remembered to add to
// a list, so a column added to one dialect and forgotten in the other passed
// silently. Deriving both sides makes drift fail the build instead.
func TestSchemaParityIsComplete(t *testing.T) {
	sqliteTables := columnsByTable(sqliteRuntimeSchema(t))
	postgresTables := columnsByTable(postgresMigrationSchema(t))

	if len(sqliteTables) == 0 || len(postgresTables) == 0 {
		t.Fatalf("failed to parse schemas (sqlite=%d tables, postgres=%d tables)", len(sqliteTables), len(postgresTables))
	}
	// Guard against a parser regression quietly emptying the comparison.
	if len(sqliteTables) < 20 {
		t.Fatalf("only %d SQLite tables parsed; the extractor is probably broken", len(sqliteTables))
	}

	for _, problem := range schemaDifferences(sqliteTables, postgresTables) {
		t.Errorf("schema drift: %s", problem)
	}
}
