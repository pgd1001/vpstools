package main

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
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

func hashCmd(cmd string) string {
	h := md5.Sum([]byte(cmd))
	return hex.EncodeToString(h[:])
}

func writeAuditEvent(ctx context.Context, db *sql.DB, orgID, actorID, action, targetType, targetID, result string, metadata map[string]any) {
	b, _ := json.Marshal(metadata)
	_, err := db.ExecContext(ctx,
		"INSERT INTO audit_events (id, organisation_id, actor_id, action, target_type, target_id, result, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		newUUID(), orgID, actorID, action, targetType, targetID, result, string(b))
	if err != nil {
		log.Printf("audit write error: %v", err)
	}
}
