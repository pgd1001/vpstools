package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, `
		INSERT INTO organisations (id, name, slug) VALUES ('org_demo', 'Demo Org', 'demo')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO users (id, email, display_name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO users (id, email, display_name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO servers (id, organisation_id, name, hostname, environment, status)
		VALUES ('srv_demo', 'org_demo', 'demo-server', 'localhost', 'development', 'pending')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO server_tags (organisation_id, server_id, key, value)
		VALUES ('org_demo', 'srv_demo', 'role', 'app')
		ON CONFLICT DO NOTHING;
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("seed completed")
}
