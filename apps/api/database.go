package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pgd1001/svrtools/packages/config"
	dbruntime "github.com/pgd1001/svrtools/packages/db"
	_ "modernc.org/sqlite"
)

const sqlitePoolSize = 1

// openMetadataDatabase opens the database used by the API metadata store.
// PostgreSQL is available to the composition layer, but the API startup guard
// remains responsible for refusing it until all handlers are dialect-safe.
func openMetadataDatabase(ctx context.Context, cfg config.BackendConfig) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch cfg.DatabaseDriver {
	case "sqlite":
		dsn := cfg.DatabaseURL + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, fmt.Errorf("open sqlite metadata database: %w", err)
		}
		db.SetMaxOpenConns(sqlitePoolSize)
		db.SetMaxIdleConns(sqlitePoolSize)
		return db, nil

	case "postgres":
		connString := strings.TrimSpace(cfg.DatabaseURL)
		if err := validatePostgresURL(connString); err != nil {
			return nil, err
		}
		pgConfig, err := pgx.ParseConfig(connString)
		if err != nil {
			return nil, fmt.Errorf("invalid PostgreSQL database URL: %w", err)
		}
		return stdlib.OpenDB(*pgConfig), nil

	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
}

func validatePostgresURL(connString string) error {
	parsed, err := url.Parse(connString)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return fmt.Errorf("invalid PostgreSQL database URL")
	}
	return nil
}

func metadataRuntime() *dbruntime.Runtime {
	driver := apiBackends.DatabaseDriver
	if driver == "" {
		driver = "sqlite"
	}
	runtime, err := dbruntime.NewRuntime(driver)
	if err != nil {
		// Configuration validation rejects this before serving requests. The
		// SQLite fallback keeps isolated unit tests using their in-memory DBs.
		runtime, _ = dbruntime.NewRuntime("sqlite")
	}
	return runtime
}
