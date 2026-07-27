package dialect

import (
	"testing"
	"time"
)

func TestParseDrivers(t *testing.T) {
	for _, test := range []struct {
		input string
		want  Driver
	}{
		{"sqlite", DriverSQLite},
		{" POSTGRES ", DriverPostgreSQL},
	} {
		got, err := Parse(test.input)
		if err != nil || got != test.want {
			t.Fatalf("Parse(%q) = %q, %v, want %q", test.input, got, err, test.want)
		}
	}
	if _, err := Parse("mysql"); err == nil {
		t.Fatal("Parse(mysql) should reject an unsupported driver")
	}
}

func TestRebindQuotedStringsEscapesAndComments(t *testing.T) {
	query := `SELECT '?', "?", [ ? ], ??, \?, ? -- ?` + "\n" + `FROM things WHERE note = 'it''s ?' /* ? */ AND id = ?`
	want := `SELECT '?', "?", [ ? ], ?, ?, $1 -- ?` + "\n" + `FROM things WHERE note = 'it''s ?' /* ? */ AND id = $2`
	if got := Rebind(DriverPostgreSQL, query); got != want {
		t.Fatalf("Rebind() = %q, want %q", got, want)
	}
}

func TestRebindCommonStatements(t *testing.T) {
	tests := []struct {
		name, query, want string
	}{
		{"insert", "INSERT INTO users (id, name) VALUES (?, ?)", "INSERT INTO users (id, name) VALUES ($1, $2)"},
		{"update", "UPDATE users SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", "UPDATE users SET name = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2"},
		{"sqlite", "INSERT INTO audit (actor_id, message) VALUES (?, ?)", "INSERT INTO audit (actor_id, message) VALUES (?, ?)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			driver := DriverPostgreSQL
			if test.name == "sqlite" {
				driver = DriverSQLite
			}
			if got := Rebind(driver, test.query); got != test.want {
				t.Fatalf("Rebind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTimeHelpers(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.FixedZone("IST", 3600))
	if CurrentTime(DriverSQLite) != "CURRENT_TIMESTAMP" || CurrentTime(DriverPostgreSQL) != "CURRENT_TIMESTAMP" {
		t.Fatal("expected portable current-time expressions")
	}
	query, args := TimeAfter(DriverSQLite, now, 90*time.Second)
	if query != "datetime(?, ?)" || len(args) != 2 || args[1] != "+90 seconds" {
		t.Fatalf("SQLite TimeAfter() = %q, %#v", query, args)
	}
	query, args = TimeAfter(DriverPostgreSQL, now, 90*time.Second)
	if query != "? + (? * INTERVAL '1 second')" || len(args) != 2 || args[1] != int64(90) {
		t.Fatalf("PostgreSQL TimeAfter() = %q, %#v", query, args)
	}
	want := time.Date(2026, 7, 27, 11, 1, 30, 0, time.UTC)
	if got := TimestampAfter(now, 90*time.Second); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("TimestampAfter() = %v, want %v UTC", got, want)
	}
}
