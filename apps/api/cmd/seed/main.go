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
		INSERT INTO organisations (id, name) VALUES ('org_demo', 'Demo Org')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO users (id, email, name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO users (id, email, name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO memberships (id, organisation_id, user_id, role)
		VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer')
		ON CONFLICT (id) DO NOTHING;

		INSERT INTO servers (id, organisation_id, name, hostname, environment, tags, status)
		VALUES ('srv_demo', 'org_demo', 'demo-server', 'localhost', 'development', '{"role":"app"}', 'unknown')
		ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("seed completed")
}
