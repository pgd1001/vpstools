package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeRawJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
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

func hashPayload(v any) string {
	b, _ := json.Marshal(v)
	return hashCmd(string(b))
}

func validIdempotencyKey(key string) bool {
	if len(key) == 0 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
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
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type auditHashInput struct {
	ID             string `json:"id"`
	OrganisationID string `json:"organisation_id"`
	ActorUserID    string `json:"actor_user_id"`
	ActorType      string `json:"actor_type"`
	Action         string `json:"action"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Result         string `json:"result"`
	Metadata       string `json:"metadata"`
	OccurredAt     string `json:"occurred_at"`
	PreviousHash   string `json:"previous_hash"`
}

func auditEventHash(input auditHashInput) string {
	return hashPayload(input)
}

func writeAuditEventTx(ctx context.Context, exec auditExec, orgID, actorID, action, targetType, targetID, result string, metadata map[string]any) error {
	return writeAuditEventTypeTx(ctx, exec, orgID, actorID, "user", action, targetType, targetID, result, metadata)
}

func writeAuditEventTypeTx(ctx context.Context, exec auditExec, orgID, actorID, actorType, action, targetType, targetID, result string, metadata map[string]any) error {
	runtime := metadataRuntime()
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	id := "aud_" + shortID()
	occurredAt := time.Now().UTC().Format("2006-01-02 15:04:05.000000")
	previousHash, err := latestAuditHash(ctx, exec, orgID)
	if err != nil {
		return err
	}
	eventHash := auditEventHash(auditHashInput{
		ID: id, OrganisationID: orgID, ActorUserID: actorID, ActorType: actorType,
		Action: action, TargetType: targetType, TargetID: targetID, Result: result,
		Metadata: string(b), OccurredAt: occurredAt, PreviousHash: previousHash,
	})
	_, err = runtime.ExecContext(ctx, exec,
		"INSERT INTO audit_events (id, organisation_id, actor_user_id, actor_type, action, target_type, target_id, result, metadata, occurred_at, previous_hash, event_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, "+runtime.JSONParameter()+", ?, ?, ?)",
		id, orgID, sqlNullString(actorID), actorType, action, targetType, targetID, result, string(b), occurredAt, previousHash, eventHash)
	return err
}

func latestAuditHash(ctx context.Context, exec auditExec, orgID string) (string, error) {
	rows, err := metadataRuntime().QueryContext(ctx, exec, "SELECT COALESCE(event_hash,'') FROM audit_events WHERE organisation_id = ? ORDER BY occurred_at DESC, id DESC LIMIT 1", orgID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return "", err
		}
		return value, nil
	}
	return "", rows.Err()
}

func backfillAuditHashChain(ctx context.Context, db *sql.DB) error {
	rows, err := metadataRuntime().QueryContext(ctx, db, `SELECT id, organisation_id, COALESCE(actor_user_id,''), actor_type, action, COALESCE(target_type,''), COALESCE(target_id,''), result, metadata, occurred_at, COALESCE(previous_hash,''), COALESCE(event_hash,'') FROM audit_events ORDER BY organisation_id, occurred_at, id`)
	if err != nil {
		return err
	}
	type record struct {
		input auditHashInput
		prev  string
		hash  string
	}
	var records []record
	for rows.Next() {
		var item record
		if err := rows.Scan(&item.input.ID, &item.input.OrganisationID, &item.input.ActorUserID, &item.input.ActorType, &item.input.Action, &item.input.TargetType, &item.input.TargetID, &item.input.Result, &item.input.Metadata, &item.input.OccurredAt, &item.input.PreviousHash, &item.hash); err != nil {
			rows.Close()
			return err
		}
		item.prev = item.input.PreviousHash
		records = append(records, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	previousByOrg := map[string]string{}
	for i := range records {
		item := &records[i]
		if item.hash == "" {
			item.input.PreviousHash = previousByOrg[item.input.OrganisationID]
			item.hash = auditEventHash(item.input)
			if _, err := metadataRuntime().ExecContext(ctx, db, "UPDATE audit_events SET previous_hash = ?, event_hash = ? WHERE id = ? AND organisation_id = ?", item.input.PreviousHash, item.hash, item.input.ID, item.input.OrganisationID); err != nil {
				return err
			}
		}
		previousByOrg[item.input.OrganisationID] = item.hash
	}
	return nil
}

func verifyAuditHashChain(ctx context.Context, db *sql.DB, orgID string) (int, error) {
	rows, err := apiQuery(ctx, db, `SELECT id, organisation_id, COALESCE(actor_user_id,''), actor_type, action, COALESCE(target_type,''), COALESCE(target_id,''), result, metadata, occurred_at, COALESCE(previous_hash,''), COALESCE(event_hash,'') FROM audit_events WHERE organisation_id = ? ORDER BY occurred_at, id`, orgID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	previous := ""
	checked := 0
	for rows.Next() {
		var input auditHashInput
		var storedHash string
		if err := rows.Scan(&input.ID, &input.OrganisationID, &input.ActorUserID, &input.ActorType, &input.Action, &input.TargetType, &input.TargetID, &input.Result, &input.Metadata, &input.OccurredAt, &input.PreviousHash, &storedHash); err != nil {
			return checked, err
		}
		if input.PreviousHash != previous || storedHash == "" || storedHash != auditEventHash(input) {
			return checked, fmt.Errorf("audit chain verification failed at event %s", input.ID)
		}
		previous = storedHash
		checked++
	}
	return checked, rows.Err()
}

func recordExecutionEvent(ctx context.Context, exec auditExec, orgID, executionID, targetID, fromStatus, toStatus, eventType string, metadata map[string]any) error {
	runtime := metadataRuntime()
	b, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = runtime.ExecContext(ctx, exec,
		"INSERT INTO execution_events (id, organisation_id, execution_id, target_id, from_status, to_status, event_type, metadata, occurred_at) VALUES (?, ?, ?, ?, ?, ?, ?, "+runtime.JSONParameter()+", "+runtime.CurrentTime()+")",
		"evt_"+shortID(), orgID, executionID, sqlNullString(targetID), sqlNullString(fromStatus), toStatus, eventType, string(b))
	return err
}
