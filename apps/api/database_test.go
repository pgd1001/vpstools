package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pgd1001/svrtools/packages/config"
)

func TestOpenMetadataDatabaseSQLiteConfiguresWALAndBoundedPool(t *testing.T) {
	db, err := openMetadataDatabase(context.Background(), config.BackendConfig{
		DatabaseDriver: "sqlite",
		DatabaseURL:    filepath.Join(t.TempDir(), "metadata.db"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if got := db.Stats().MaxOpenConnections; got != sqlitePoolSize {
		t.Fatalf("max open connections = %d, want %d", got, sqlitePoolSize)
	}
	if got, want := pragmaValue(t, db, "journal_mode"), "wal"; got != want {
		t.Fatalf("journal_mode = %q, want %q", got, want)
	}
	if got, want := pragmaValue(t, db, "foreign_keys"), "1"; got != want {
		t.Fatalf("foreign_keys = %q, want %q", got, want)
	}
	if got, want := pragmaValue(t, db, "busy_timeout"), "5000"; got != want {
		t.Fatalf("busy_timeout = %q, want %q", got, want)
	}
}

func TestOpenMetadataDatabasePostgresValidatesWithoutSQLiteFallback(t *testing.T) {
	db, err := openMetadataDatabase(context.Background(), config.BackendConfig{
		DatabaseDriver: "postgres",
		DatabaseURL:    "postgresql://user:secret@localhost:5432/metadata?sslmode=disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, ok := db.Driver().(*stdlib.Driver); !ok {
		t.Fatalf("database driver = %T, want pgx stdlib driver", db.Driver())
	}
}

func TestOpenMetadataDatabaseRejectsInvalidPostgresURL(t *testing.T) {
	for _, databaseURL := range []string{"", "./metadata.db", "mysql://localhost/metadata", "postgresql://"} {
		t.Run(databaseURL, func(t *testing.T) {
			db, err := openMetadataDatabase(context.Background(), config.BackendConfig{
				DatabaseDriver: "postgres",
				DatabaseURL:    databaseURL,
			})
			if err == nil {
				db.Close()
				t.Fatal("expected invalid PostgreSQL URL to fail")
			}
		})
	}
}

func TestValidatePostgresSchemaColumnsAcceptsHandlerContract(t *testing.T) {
	var columns []postgresSchemaColumn
	for table, required := range postgresSchemaContract {
		for _, column := range required {
			columns = append(columns, postgresSchemaColumn{Table: table, Column: column})
		}
	}

	if err := validatePostgresSchemaColumns(columns); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePostgresSchemaColumnsReportsAllMissingColumns(t *testing.T) {
	err := validatePostgresSchemaColumns([]postgresSchemaColumn{
		{Table: "users", Column: "id"},
		{Table: "organisations", Column: "id"},
	})
	if err == nil {
		t.Fatal("expected schema validation to fail")
	}
	message := err.Error()
	for _, want := range []string{"organisations.name", "users.email", "servers.id"} {
		if !strings.Contains(message, want) {
			t.Fatalf("schema error %q does not contain %q", message, want)
		}
	}
}

func pragmaValue(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var value string
	if err := db.QueryRow("PRAGMA " + name).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
