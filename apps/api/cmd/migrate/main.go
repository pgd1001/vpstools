package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
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

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("failed to set dialect: %v", err)
	}

	migrationsDir := "migrations/postgres"

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
