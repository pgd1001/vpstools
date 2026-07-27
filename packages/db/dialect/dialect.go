// Package dialect contains the small amount of SQL that differs between the
// database engines supported by the planned control plane. It is deliberately
// independent of database drivers and does not claim that either runtime is
// wired to both engines yet.
package dialect

import (
	"fmt"
	"strings"
	"time"
)

// Driver is the database/sql driver identifier used by the application
// configuration. PostgreSQL is named "postgres" to match the existing
// configuration and migration tooling.
type Driver string

const (
	DriverSQLite     Driver = "sqlite"
	DriverPostgreSQL Driver = "postgres"

	// DriverPostgres is kept as a short spelling for callers that use the
	// database name rather than its configuration identifier.
	DriverPostgres = DriverPostgreSQL
)

// Parse validates a configured driver identifier.
func Parse(name string) (Driver, error) {
	driver := Driver(strings.ToLower(strings.TrimSpace(name)))
	switch driver {
	case DriverSQLite, DriverPostgreSQL:
		return driver, nil
	default:
		return "", fmt.Errorf("unsupported database driver %q", name)
	}
}

// Rebind changes ? parameters to the placeholder syntax for driver. SQL
// strings, quoted identifiers, comments, ??, and \? are left as literals.
// The input uses ? parameters so query text can be shared by both dialects.
func Rebind(driver Driver, query string) string {
	if driver != DriverSQLite && driver != DriverPostgreSQL {
		return query
	}
	postgres := driver == DriverPostgreSQL

	var out strings.Builder
	out.Grow(len(query) + 8)
	parameter := 1
	for i := 0; i < len(query); {
		switch query[i] {
		case '\'', '"', '`':
			i = copyQuoted(&out, query, i, query[i])
		case '[':
			i = copyBracketIdentifier(&out, query, i)
		case '-':
			if i+1 < len(query) && query[i+1] == '-' {
				i = copyLineComment(&out, query, i)
			} else {
				out.WriteByte(query[i])
				i++
			}
		case '/':
			if i+1 < len(query) && query[i+1] == '*' {
				i = copyBlockComment(&out, query, i)
			} else {
				out.WriteByte(query[i])
				i++
			}
		case '?':
			if i+1 < len(query) && query[i+1] == '?' {
				out.WriteByte('?')
				i += 2
			} else if postgres {
				out.WriteString(fmt.Sprintf("$%d", parameter))
				parameter++
				i++
			} else {
				out.WriteByte('?')
				i++
			}
		case '\\':
			if i+1 < len(query) && query[i+1] == '?' {
				out.WriteByte('?')
				i += 2
			} else {
				out.WriteByte(query[i])
				i++
			}
		default:
			out.WriteByte(query[i])
			i++
		}
	}
	return out.String()
}

// CurrentTime returns a server-side current-time expression supported by the
// selected dialect. Keep the expression free of user input.
func CurrentTime(driver Driver) string {
	switch driver {
	case DriverSQLite, DriverPostgreSQL:
		return "CURRENT_TIMESTAMP"
	default:
		return ""
	}
}

// TimeAfter returns a parameterized expression and its arguments for a time
// interval from now. Call Rebind before executing the expression. The caller
// may prefer TimestampAfter when server clock consistency is not required.
func TimeAfter(driver Driver, now time.Time, interval time.Duration) (string, []any) {
	now = now.UTC()
	seconds := int64(interval / time.Second)
	switch driver {
	case DriverSQLite:
		return "datetime(?, ?)", []any{now, fmt.Sprintf("%+d seconds", seconds)}
	case DriverPostgreSQL:
		return "? + (? * INTERVAL '1 second')", []any{now, seconds}
	default:
		return "", nil
	}
}

// TimestampAfter computes a timestamp in Go, avoiding dialect-specific
// interval syntax when the application already has an appropriate clock.
func TimestampAfter(now time.Time, interval time.Duration) time.Time {
	return now.UTC().Add(interval)
}

func copyQuoted(out *strings.Builder, query string, start int, quote byte) int {
	out.WriteByte(query[start])
	for i := start + 1; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] == quote {
			if i+1 < len(query) && query[i+1] == quote {
				out.WriteByte(query[i+1])
				i++
				continue
			}
			return i + 1
		}
	}
	return len(query)
}

func copyBracketIdentifier(out *strings.Builder, query string, start int) int {
	for i := start; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] == ']' {
			return i + 1
		}
	}
	return len(query)
}

func copyLineComment(out *strings.Builder, query string, start int) int {
	for i := start; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] == '\n' {
			return i + 1
		}
	}
	return len(query)
}

func copyBlockComment(out *strings.Builder, query string, start int) int {
	for i := start; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] == '*' && i+1 < len(query) && query[i+1] == '/' {
			out.WriteByte('/')
			return i + 2
		}
	}
	return len(query)
}
