package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"os"

	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func newUUID() string {
	return uuid.New().String()
}

func shortID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 10)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func hashCmd(cmd string) string {
	h := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(h[:])
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeAuditEvent(ctx context.Context, db *sql.DB, orgID, actorID, action, targetType, targetID, result string, metadata map[string]any) {
	err := writeAuditEventTypeTx(ctx, db, orgID, actorID, "user", action, targetType, targetID, result, metadata)
	if err != nil {
		slog.Error("audit write error", "error", err)
	}
}

type auditExec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func writeAuditEventTx(ctx context.Context, exec auditExec, orgID, actorID, action, targetType, targetID, result string, metadata map[string]any) error {
	return writeAuditEventTypeTx(ctx, exec, orgID, actorID, "user", action, targetType, targetID, result, metadata)
}

func writeAuditEventTypeTx(ctx context.Context, exec auditExec, orgID, actorID, actorType, action, targetType, targetID, result string, metadata map[string]any) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx,
		"INSERT INTO audit_events (id, organisation_id, actor_user_id, actor_type, action, target_type, target_id, result, metadata, occurred_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))",
		"aud_"+shortID(), orgID, sqlNullString(actorID), actorType, action, targetType, targetID, result, string(b))
	return err
}

func recordExecutionEvent(ctx context.Context, exec auditExec, orgID, executionID, targetID, fromStatus, toStatus, eventType string, metadata map[string]any) error {
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx,
		"INSERT INTO execution_events (id, organisation_id, execution_id, target_id, from_status, to_status, event_type, metadata, occurred_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))",
		"evt_"+shortID(), orgID, executionID, sqlNullString(targetID), sqlNullString(fromStatus), toStatus, eventType, string(b))
	return err
}
