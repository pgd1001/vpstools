package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

// TestPostgresRuntimeIntegration is opt-in because the default development
// environment is deliberately self-contained. CI or a release rehearsal can
// set POSTGRES_TEST_URL to run the real migration and catalog contract against
// a disposable PostgreSQL instance.
func TestPostgresRuntimeIntegration(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpContext(ctx, db, postgresIntegrationMigrationPath(t)); err != nil {
		t.Fatal(err)
	}
	if err := verifyPostgresSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
}

func postgresIntegrationMigrationPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test source")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations", "postgres")
}
