package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status|reset>")
		os.Exit(1)
	}
	action := os.Args[1]

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/svrtools?sslmode=disable"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("failed to verify PostgreSQL connection: %v", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("failed to set dialect: %v", err)
	}

	migrationsDir, err := migrationDir()
	if err != nil {
		log.Fatal(err)
	}

	switch action {
	case "up":
		if err := goose.Up(db, migrationsDir); err != nil {
			log.Fatalf("migration up failed: %v", err)
		}
		fmt.Println("migrations applied")
	case "down":
		if err := goose.Down(db, migrationsDir); err != nil {
			log.Fatalf("migration down failed: %v", err)
		}
		fmt.Println("migration rolled back")
	case "reset":
		if err := goose.Reset(db, migrationsDir); err != nil {
			log.Fatalf("migration reset failed: %v", err)
		}
		fmt.Println("database reset")
	case "status":
		if err := goose.Status(db, migrationsDir); err != nil {
			log.Fatalf("migration status failed: %v", err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown action: %s\n", action)
		os.Exit(1)
	}
}

func migrationDir() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to locate migration source")
	}
	for _, start := range []string{filepath.Dir(source), workingDirectory(), executableDirectory()} {
		if root, ok := projectRoot(start); ok {
			return filepath.Join(root, "migrations", "postgres"), nil
		}
	}
	return "", fmt.Errorf("unable to locate migrations/postgres from source, working directory, or executable")
}

func projectRoot(start string) (string, bool) {
	path, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		migrationPath := filepath.Join(path, "migrations", "postgres")
		if info, err := os.Stat(migrationPath); err == nil && info.IsDir() {
			return path, true
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", false
		}
		path = parent
	}
}

func workingDirectory() string {
	path, err := os.Getwd()
	if err != nil {
		return ""
	}
	return path
}

func executableDirectory() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(path)
}
