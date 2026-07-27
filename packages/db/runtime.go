package db

import (
	"context"
	"database/sql"

	"github.com/pgd1001/svrtools/packages/db/dialect"
)

// Runtime is the small execution boundary shared by database-backed code. SQL
// is authored with ? placeholders and the runtime binds it for the selected
// driver at the point of execution.
type Runtime struct {
	Driver dialect.Driver
}

func NewRuntime(driver string) (*Runtime, error) {
	d, err := dialect.Parse(driver)
	if err != nil {
		return nil, err
	}
	return &Runtime{Driver: d}, nil
}

func (r Runtime) Rebind(query string) string { return dialect.Rebind(r.Driver, query) }

type Execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type QueryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r Runtime) ExecContext(ctx context.Context, execer Execer, query string, args ...any) (sql.Result, error) {
	return execer.ExecContext(ctx, r.Rebind(query), args...)
}

// CheckedExecContext executes an update whose affected-row count is part of
// its correctness, such as a lease renewal or queue claim.
func (r Runtime) CheckedExecContext(ctx context.Context, execer Execer, query string, args ...any) (bool, error) {
	result, err := r.ExecContext(ctx, execer, query, args...)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (r Runtime) QueryContext(ctx context.Context, queryer Queryer, query string, args ...any) (*sql.Rows, error) {
	return queryer.QueryContext(ctx, r.Rebind(query), args...)
}

func (r Runtime) QueryRowContext(ctx context.Context, queryer QueryRower, query string, args ...any) *sql.Row {
	return queryer.QueryRowContext(ctx, r.Rebind(query), args...)
}

func (r Runtime) CurrentTime() string { return dialect.CurrentTime(r.Driver) }

// JSONParameter returns a parameter expression suitable for JSON/JSONB
// columns. The caller still passes the serialized JSON as an argument.
func (r Runtime) JSONParameter() string {
	if r.Driver == dialect.DriverPostgreSQL {
		return "?::jsonb"
	}
	return "?"
}
