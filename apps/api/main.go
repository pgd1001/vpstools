package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	dbPath := envOrDefault("DB_PATH", "svrtools.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		log.Fatalf("database open failed: %v", err)
	}
	defer db.Close()

	if err := migrate(ctx, db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	if err := seed(ctx, db); err != nil {
		log.Fatalf("seed failed: %v", err)
	}

	log.Println("database ready")

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{
			"status":  "ok",
			"version": "0.1.0",
		})
	})

	mux.HandleFunc("/api/v1/whoami", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{
			"user_id":         "user_senior",
			"email":           "senior@demo.local",
			"name":            "Senior Engineer",
			"organisation_id": "org_demo",
			"organisation":    "Demo Org",
			"role":            "senior_engineer",
		})
	})

	mux.HandleFunc("/api/v1/servers", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(ctx, "SELECT id, name, hostname, environment, tags, status FROM servers WHERE organisation_id = ?", "org_demo")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()

		type server struct {
			ID          string            `json:"id"`
			Name        string            `json:"name"`
			Hostname    string            `json:"hostname"`
			Environment string            `json:"environment"`
			Tags        map[string]string `json:"tags"`
			Status      string            `json:"status"`
		}
		var servers []server
		for rows.Next() {
			var s server
			var tags string
			if err := rows.Scan(&s.ID, &s.Name, &s.Hostname, &s.Environment, &tags, &s.Status); err != nil {
				continue
			}
			json.Unmarshal([]byte(tags), &s.Tags)
			servers = append(servers, s)
		}
		writeJSON(w, 200, map[string]any{"servers": servers})
	})

	mux.HandleFunc("/api/v1/executions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var req struct {
			Target  string `json:"target"`
			Command string `json:"command"`
			Reason  string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}

		execID := newUUID()
		_, err := db.ExecContext(ctx,
			"INSERT INTO executions (id, organisation_id, actor_id, command, command_hash, status, reason) VALUES (?, ?, ?, ?, ?, ?, ?)",
			execID, "org_demo", "user_senior", req.Command, hashCmd(req.Command), "queued", req.Reason,
		)
		if err != nil {
			log.Printf("execution create error: %v", err)
			writeJSON(w, 500, map[string]string{"error": "failed to create execution"})
			return
		}

		writeAuditEvent(ctx, db, "org_demo", "user_senior", "execution.created", "server", req.Target, "queued", map[string]any{
			"command": req.Command,
			"reason":  req.Reason,
			"target":  req.Target,
		})

		writeJSON(w, 201, map[string]any{"execution_id": execID, "status": "queued"})
	})

	mux.HandleFunc("/api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		var execID, command string
		err := db.QueryRowContext(ctx,
			`UPDATE executions SET status = 'running', started_at = datetime('now')
			WHERE id = (SELECT id FROM executions WHERE status = 'queued' ORDER BY created_at ASC LIMIT 1)
			RETURNING id, command`,
		).Scan(&execID, &command)
		if err != nil {
			writeJSON(w, 404, map[string]string{"status": "no_jobs"})
			return
		}
		writeJSON(w, 200, map[string]any{
			"execution_id": execID,
			"command":      command,
		})
	})

	mux.HandleFunc("/api/v1/jobs/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var req struct {
			ExecutionID string `json:"execution_id"`
			ExitCode    int    `json:"exit_code"`
			Stdout      string `json:"stdout"`
			Stderr      string `json:"stderr"`
			Error       string `json:"error"`
			DurationMs  int64  `json:"duration_ms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid request body"})
			return
		}

		status := "succeeded"
		if req.ExitCode != 0 || req.Error != "" {
			status = "failed"
		}

		_, err := db.ExecContext(ctx,
			"UPDATE executions SET status = ?, finished_at = datetime('now') WHERE id = ?",
			status, req.ExecutionID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to update execution"})
			return
		}

		writeAuditEvent(ctx, db, "org_demo", "user_senior", "execution.completed", "execution", req.ExecutionID, status, map[string]any{
			"exit_code":   req.ExitCode,
			"duration_ms": req.DurationMs,
		})

		writeJSON(w, 200, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		actor := r.URL.Query().Get("actor")
		limit := "20"
		if l := r.URL.Query().Get("limit"); l != "" {
			limit = l
		}
		query := "SELECT id, organisation_id, actor_id, action, target_type, target_id, result, metadata_json, created_at FROM audit_events WHERE 1=1"
		var args []any
		if actor != "" {
			query += " AND actor_id = ?"
			args = append(args, actor)
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()

		var events []map[string]any
		for rows.Next() {
			var id, orgID, actorID, action, targetType, targetID, result, metadataJSON string
			var createdAt string
			if err := rows.Scan(&id, &orgID, &actorID, &action, &targetType, &targetID, &result, &metadataJSON, &createdAt); err != nil {
				continue
			}
			events = append(events, map[string]any{
				"id":              id,
				"organisation_id": orgID,
				"actor_id":        actorID,
				"action":          action,
				"target_type":     targetType,
				"target_id":       targetID,
				"result":          result,
				"metadata_json":   metadataJSON,
				"created_at":      createdAt,
			})
		}
		writeJSON(w, 200, map[string]any{"events": events})
	})

	addr := ":" + envOrDefault("API_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	srv.Shutdown(shutdownCtx)
}

func migrate(ctx context.Context, db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS organisations (
		id TEXT PRIMARY KEY, name TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY, email TEXT NOT NULL, name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active', created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS memberships (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		user_id TEXT NOT NULL REFERENCES users(id), role TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS servers (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		name TEXT NOT NULL, hostname TEXT NOT NULL,
		environment TEXT NOT NULL DEFAULT 'staging',
		tags TEXT NOT NULL DEFAULT '{}', status TEXT NOT NULL DEFAULT 'unknown',
		last_seen_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS executions (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		actor_id TEXT NOT NULL REFERENCES users(id),
		command TEXT NOT NULL, command_hash TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued', reason TEXT,
		dry_run INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		started_at TEXT, finished_at TEXT
	);
	CREATE TABLE IF NOT EXISTS execution_targets (
		id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(id),
		server_id TEXT NOT NULL REFERENCES servers(id),
		status TEXT NOT NULL DEFAULT 'pending', exit_code INTEGER,
		stdout TEXT, stderr TEXT, error TEXT, duration_ms INTEGER,
		started_at TEXT, finished_at TEXT
	);
	CREATE TABLE IF NOT EXISTS audit_events (
		id TEXT PRIMARY KEY, organisation_id TEXT NOT NULL REFERENCES organisations(id),
		actor_id TEXT, action TEXT NOT NULL, target_type TEXT NOT NULL,
		target_id TEXT, result TEXT NOT NULL,
		metadata_json TEXT NOT NULL DEFAULT '{}',
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_audit_events_org_time ON audit_events(organisation_id, created_at);
	CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor_id);
	CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(action);
	CREATE INDEX IF NOT EXISTS idx_executions_org_status ON executions(organisation_id, status);
	CREATE INDEX IF NOT EXISTS idx_servers_org ON servers(organisation_id);
	`
	_, err := db.ExecContext(ctx, schema)
	return err
}

func seed(ctx context.Context, db *sql.DB) error {
	stmt := `
	INSERT OR IGNORE INTO organisations (id, name) VALUES ('org_demo', 'Demo Org');
	INSERT OR IGNORE INTO users (id, email, name) VALUES ('user_senior', 'senior@demo.local', 'Senior Engineer');
	INSERT OR IGNORE INTO users (id, email, name) VALUES ('user_junior', 'junior@demo.local', 'Junior Engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_senior', 'org_demo', 'user_senior', 'senior_engineer');
	INSERT OR IGNORE INTO memberships (id, organisation_id, user_id, role) VALUES ('mem_junior', 'org_demo', 'user_junior', 'junior_engineer');
	INSERT OR IGNORE INTO servers (id, organisation_id, name, hostname, environment, tags, status) VALUES ('srv_demo', 'org_demo', 'demo-server', 'localhost', 'development', '{"role":"app"}', 'unknown');
	`
	_, err := db.ExecContext(ctx, stmt)
	return err
}
