package main

import (
	"context"
	"database/sql"
	"fmt"
)

type tenantConnectionKey struct{}

// bindTenantConnection pins a PostgreSQL request to one connection and sets
// the transaction-independent tenant GUC used by RLS policies. The connection
// is returned only after the GUC is cleared, so tenant state cannot bleed into
// another pooled request.
func bindTenantConnection(ctx context.Context, db *sql.DB, organisationID string) (context.Context, func(), error) {
	if apiBackends.DatabaseDriver != "postgres" || !apiBackends.PostgresRLS {
		return ctx, func() {}, nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return ctx, nil, fmt.Errorf("pin PostgreSQL tenant connection: %w", err)
	}
	if _, err := metadataRuntime().ExecContext(ctx, conn, "SELECT set_config('vps.organisation_id', ?, false)", organisationID); err != nil {
		_ = conn.Close()
		return ctx, nil, fmt.Errorf("set PostgreSQL tenant context: %w", err)
	}
	cleanup := func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT set_config('vps.organisation_id', '', false)")
		_ = conn.Close()
	}
	return context.WithValue(ctx, tenantConnectionKey{}, conn), cleanup, nil
}

func tenantConnection(ctx context.Context) *sql.Conn {
	conn, _ := ctx.Value(tenantConnectionKey{}).(*sql.Conn)
	return conn
}

func beginAPITx(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
	if conn := tenantConnection(ctx); conn != nil {
		return conn.BeginTx(ctx, nil)
	}
	return db.BeginTx(ctx, nil)
}
