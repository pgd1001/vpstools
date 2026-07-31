package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"runtime"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pgd1001/svrtools/packages/config"
	dbruntime "github.com/pgd1001/svrtools/packages/db"
	_ "modernc.org/sqlite"
)

// SQLite allows a single writer but many concurrent readers when the database
// is in WAL mode. Serialising reads behind the writer would make every runner
// poll contend with every dashboard query, so the API keeps two pools:
//
//   - the write pool has exactly one connection, which is what SQLite's single
//     writer actually permits and which avoids SQLITE_BUSY between writers;
//   - the read pool is sized to the machine, because WAL readers do not block
//     each other or the writer.
//
// Readers observe the last committed transaction, and every handler commits
// before responding, so a read that follows a write still sees it.
const sqliteWritePoolSize = 1

// sqliteReadPoolSize bounds concurrent readers. It is capped because each
// connection holds its own page cache and file handle.
func sqliteReadPoolSize() int {
	const minimum, maximum = 4, 16
	size := runtime.NumCPU() * 2
	if size < minimum {
		return minimum
	}
	if size > maximum {
		return maximum
	}
	return size
}

// sqlitePoolSize is retained as the write pool size under its original name so
// existing callers and assertions keep their meaning.
const sqlitePoolSize = sqliteWritePoolSize

// openMetadataDatabase opens the write handle for the API metadata store.
func openMetadataDatabase(ctx context.Context, cfg config.BackendConfig) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch cfg.DatabaseDriver {
	case "sqlite":
		db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabaseURL))
		if err != nil {
			return nil, fmt.Errorf("open sqlite metadata database: %w", err)
		}
		db.SetMaxOpenConns(sqliteWritePoolSize)
		db.SetMaxIdleConns(sqliteWritePoolSize)
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

// openMetadataReadDatabase opens a read-oriented handle for the metadata
// store. For SQLite this is a separate, larger pool so read traffic does not
// queue behind the single writer. PostgreSQL pools connections itself, so it
// returns nil and callers fall back to the primary handle.
func openMetadataReadDatabase(ctx context.Context, cfg config.BackendConfig) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.DatabaseDriver != "sqlite" {
		return nil, nil
	}
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabaseURL))
	if err != nil {
		return nil, fmt.Errorf("open sqlite read pool: %w", err)
	}
	size := sqliteReadPoolSize()
	db.SetMaxOpenConns(size)
	db.SetMaxIdleConns(size)
	return db, nil
}

// sqliteDSN applies the pragmas the control plane depends on. WAL is what makes
// concurrent reads possible; busy_timeout keeps a brief writer overlap from
// surfacing as an error.
func sqliteDSN(databaseURL string) string {
	return databaseURL + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
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
	dialectRuntime, err := dbruntime.NewRuntime(driver)
	if err != nil {
		// Configuration validation rejects this before serving requests. The
		// SQLite fallback keeps isolated unit tests using their in-memory DBs.
		dialectRuntime, _ = dbruntime.NewRuntime("sqlite")
	}
	return dialectRuntime
}
