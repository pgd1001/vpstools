package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

var validRoles = map[string]bool{
	"owner": true, "admin": true, "senior_engineer": true,
	"junior_engineer": true, "auditor": true,
}

func main() {
	dbPath := flag.String("db", envOrDefault("DB_PATH", "svrtools.db"), "SQLite database path")
	orgID := flag.String("org", "org_demo", "organisation ID")
	userID := flag.String("user-id", "", "local user ID")
	email := flag.String("email", "", "exact ZITADEL email address")
	displayName := flag.String("display-name", "", "display name")
	role := flag.String("role", "", "VPS Tools role")
	flag.Parse()

	if *userID == "" || *email == "" || *displayName == "" || !validRoles[*role] {
		fmt.Fprintln(os.Stderr, "required: --user-id, --email, --display-name, and a valid --role")
		fmt.Fprintln(os.Stderr, "valid roles: owner, admin, senior_engineer, junior_engineer, auditor")
		os.Exit(2)
	}

	db, err := sql.Open("sqlite", *dbPath+"?_pragma=foreign_keys(on)")
	if err != nil {
		fail(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		fail(fmt.Errorf("unable to verify SQLite connection: %w", err))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fail(err)
	}
	defer tx.Rollback()

	var existingID string
	err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", strings.TrimSpace(*email)).Scan(&existingID)
	if err == sql.ErrNoRows {
		_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, display_name, user_type, status) VALUES (?, ?, ?, 'human', 'active')`, *userID, strings.TrimSpace(*email), *displayName)
	} else if err == nil {
		*userID = existingID
		_, err = tx.ExecContext(ctx, `UPDATE users SET display_name = ?, status = 'active' WHERE id = ?`, *displayName, *userID)
	}
	if err != nil {
		fail(err)
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO memberships (id, organisation_id, user_id, role, status) VALUES (?, ?, ?, ?, 'active') ON CONFLICT(organisation_id, user_id) DO UPDATE SET role = excluded.role, status = 'active'`, "mem_oidc_"+strings.ReplaceAll(*userID, "-", "_"), *orgID, *userID, *role)
	if err != nil {
		fail(err)
	}
	if err = tx.Commit(); err != nil {
		fail(err)
	}
	fmt.Printf("provisioned %s as %s in %s\n", *email, *role, *orgID)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
