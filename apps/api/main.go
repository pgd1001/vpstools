package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

type tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dbPath := envOrDefault("DB_PATH", "svrtools.db")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		logger.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := migrate(ctx, db); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if err := seed(ctx, db); err != nil {
		logger.Error("seed failed", "error", err)
		os.Exit(1)
	}

	logger.Info("database ready")

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok", "version": "0.2.0"})
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
		switch r.Method {
		case http.MethodGet:
			handleListServers(ctx, db, w, r)
		case http.MethodPost:
			handleAddServer(ctx, db, w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/v1/servers/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/servers/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing server id"})
			return
		}
		if hasSuffix(path, "/check") {
			serverID := path[:len(path)-len("/check")]
			if r.Method == http.MethodPost {
				handleCheckServer(ctx, db, w, r, serverID, logger)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		switch r.Method {
		case http.MethodGet:
			handleGetServer(ctx, db, w, r, path)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/v1/runners", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListRunners(ctx, db, w, r)
		case http.MethodPost:
			handleRegisterRunner(ctx, db, w, r, logger)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/v1/runners/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleRunnerHeartbeat(ctx, db, w, r, logger)
	})

	mux.HandleFunc("/api/v1/runners/registration-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleCreateRegistrationToken(ctx, db, w, r, logger)
	})

	mux.HandleFunc("/api/v1/executions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListExecutions(ctx, db, w, r)
		case http.MethodPost:
			handleCreateExecution(ctx, db, w, r, logger)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/v1/executions/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/executions/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing execution id"})
			return
		}
		if hasSuffix(path, "/cancel") {
			execID := path[:len(path)-len("/cancel")]
			if r.Method == http.MethodPost {
				handleCancelExecution(ctx, db, w, r, execID, logger)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetExecution(ctx, db, w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	})

	mux.HandleFunc("/api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		handleClaimJob(ctx, db, w, r)
	})

	mux.HandleFunc("/api/v1/jobs/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleSubmitResult(ctx, db, w, r, logger)
	})

	mux.HandleFunc("/api/v1/audit", func(w http.ResponseWriter, r *http.Request) {
		handleSearchAudit(ctx, db, w, r)
	})

	addr := ":" + envOrDefault("API_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		logger.Info("API listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	srv.Shutdown(shutdownCtx)
}

func handleListServers(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	env := r.URL.Query().Get("environment")
	tagKey := r.URL.Query().Get("tag_key")
	tagValue := r.URL.Query().Get("tag_value")

	query := `SELECT s.id, s.name, s.hostname, COALESCE(s.public_ip,''), COALESCE(s.private_ip,''),
		s.ssh_port, COALESCE(s.ssh_username,''), s.environment, COALESCE(s.provider,''),
		COALESCE(s.os_name,''), COALESCE(s.os_version,''), COALESCE(s.kernel_version,''),
		COALESCE(s.architecture,''), s.status, COALESCE(s.last_seen_at,''),
		COALESCE(s.last_check_at,''), s.created_at
		FROM servers s WHERE s.organisation_id = ?`
	args := []any{"org_demo"}

	if env != "" {
		query += " AND s.environment = ?"
		args = append(args, env)
	}
	if tagKey != "" && tagValue != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?)`
		args = append(args, "org_demo", tagKey, tagValue)
	} else if tagKey != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ?)`
		args = append(args, "org_demo", tagKey)
	}

	query += " ORDER BY s.name ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type server struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Hostname     string   `json:"hostname"`
		PublicIP     string   `json:"public_ip"`
		PrivateIP    string   `json:"private_ip"`
		SSHPort      int      `json:"ssh_port"`
		SSHUsername  string   `json:"ssh_username"`
		Environment  string   `json:"environment"`
		Provider     string   `json:"provider"`
		OSName       string   `json:"os_name"`
		OSVersion    string   `json:"os_version"`
		Kernel       string   `json:"kernel_version"`
		Arch         string   `json:"architecture"`
		Status       string   `json:"status"`
		LastSeenAt   string   `json:"last_seen_at"`
		LastCheckAt  string   `json:"last_check_at"`
		CreatedAt    string   `json:"created_at"`
		Tags         []tag    `json:"tags"`
	}

	servers := []server{}
	for rows.Next() {
		var s server
		if err := rows.Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
			&s.SSHPort, &s.SSHUsername, &s.Environment, &s.Provider,
			&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
			&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt); err != nil {
			continue
		}
		s.Tags = loadTags(ctx, db, "org_demo", s.ID)
		servers = append(servers, s)
	}
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func handleAddServer(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Hostname    string `json:"hostname"`
		PublicIP    string `json:"public_ip"`
		PrivateIP   string `json:"private_ip"`
		SSHPort     int    `json:"ssh_port"`
		SSHUsername string `json:"ssh_username"`
		Environment string `json:"environment"`
		Provider    string `json:"provider"`
		Tags        []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.Environment == "" {
		req.Environment = "development"
	}

	serverID := "srv_" + shortID()

	pubIP := sqlNullString(req.PublicIP)
	privIP := sqlNullString(req.PrivateIP)
	sshUser := sqlNullString(req.SSHUsername)
	host := sqlNullString(req.Hostname)
	prov := sqlNullString(req.Provider)

	_, err := db.ExecContext(ctx,
		`INSERT INTO servers (id, organisation_id, name, hostname, public_ip, private_ip, ssh_port, ssh_username, environment, provider)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		serverID, "org_demo", req.Name, host, pubIP, privIP, req.SSHPort, sshUser, req.Environment, prov)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create server: " + err.Error()})
		return
	}

	for _, t := range req.Tags {
		if t.Key != "" {
			db.ExecContext(ctx,
				"INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)",
				"org_demo", serverID, t.Key, t.Value)
		}
	}

	writeAuditEvent(ctx, db, "org_demo", "user_senior", "server.created", "server", serverID, "success", map[string]any{"name": req.Name, "environment": req.Environment})

	writeJSON(w, 201, map[string]any{"server_id": serverID, "status": "created"})
}

func handleGetServer(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, serverID string) {
	s := struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Hostname    string `json:"hostname"`
		PublicIP    string `json:"public_ip"`
		PrivateIP   string `json:"private_ip"`
		SSHPort     int    `json:"ssh_port"`
		SSHUsername string `json:"ssh_username"`
		Environment string `json:"environment"`
		Provider    string `json:"provider"`
		OSName      string `json:"os_name"`
		OSVersion   string `json:"os_version"`
		Kernel      string `json:"kernel_version"`
		Arch        string `json:"architecture"`
		Status      string `json:"status"`
		LastSeenAt  string `json:"last_seen_at"`
		LastCheckAt string `json:"last_check_at"`
		CreatedAt   string `json:"created_at"`
		Tags        []tag `json:"tags"`
	}{
		Tags: loadTags(ctx, db, "org_demo", serverID),
	}

	err := db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(hostname,''), COALESCE(public_ip,''), COALESCE(private_ip,''),
		ssh_port, COALESCE(ssh_username,''), environment, COALESCE(provider,''),
		COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''),
		COALESCE(architecture,''), status, COALESCE(last_seen_at,''),
		COALESCE(last_check_at,''), created_at
		FROM servers WHERE id = ? AND organisation_id = ?`, serverID, "org_demo",
	).Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
		&s.SSHPort, &s.SSHUsername, &s.Environment, &s.Provider,
		&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
		&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"server": s})
}

func handleCheckServer(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, serverID string, logger *slog.Logger) {
	var host, sshUser string
	var sshPort int
	err := db.QueryRowContext(ctx,
		"SELECT COALESCE(hostname, public_ip, ''), ssh_port, COALESCE(ssh_username,'') FROM servers WHERE id = ? AND organisation_id = ?",
		serverID, "org_demo").Scan(&host, &sshPort, &sshUser)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.ExecContext(ctx,
		"UPDATE servers SET status = 'active', last_check_at = ?, last_seen_at = ? WHERE id = ?",
		now, now, serverID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update server"})
		return
	}

	checkResult := map[string]any{
		"server_id":  serverID,
		"status":     "reachable",
		"hostname":   host,
		"ssh_port":   sshPort,
		"checked_at": now,
	}

	// Simulate OS info if we don't have it yet
	var osName, osVer, kernel, arch string
	db.QueryRowContext(ctx,
		"SELECT COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''), COALESCE(architecture,'') FROM servers WHERE id = ?",
		serverID).Scan(&osName, &osVer, &kernel, &arch)

	if osName == "" {
		osName = "linux"
		osVer = "unknown"
		kernel = "unknown"
		arch = "amd64"
		db.ExecContext(ctx,
			"UPDATE servers SET os_name=?, os_version=?, kernel_version=?, architecture=? WHERE id=?",
			osName, osVer, kernel, arch, serverID)
	}
	checkResult["os_name"] = osName
	checkResult["os_version"] = osVer
	checkResult["kernel_version"] = kernel
	checkResult["architecture"] = arch
	checkResult["uptime"] = "0d 0h 0m (simulated)"

	writeJSON(w, 200, map[string]any{"server": checkResult})
}

func handleListRunners(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, runner_type, status, COALESCE(version,''), COALESCE(hostname,''),
		COALESCE(platform,''), COALESCE(ip_address,''), COALESCE(last_seen_at,''),
		COALESCE(registered_at,''), COALESCE(revoked_at,''), created_at
		FROM runners WHERE organisation_id = ? ORDER BY name ASC`, "org_demo")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type runner struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		RunnerType   string `json:"runner_type"`
		Status       string `json:"status"`
		Version      string `json:"version"`
		Hostname     string `json:"hostname"`
		Platform     string `json:"platform"`
		IPAddress    string `json:"ip_address"`
		LastSeenAt   string `json:"last_seen_at"`
		RegisteredAt string `json:"registered_at"`
		RevokedAt    string `json:"revoked_at"`
		CreatedAt    string `json:"created_at"`
	}

	results := []runner{}
	for rows.Next() {
		var r runner
		rows.Scan(&r.ID, &r.Name, &r.RunnerType, &r.Status, &r.Version,
			&r.Hostname, &r.Platform, &r.IPAddress, &r.LastSeenAt,
			&r.RegisteredAt, &r.RevokedAt, &r.CreatedAt)
		results = append(results, r)
	}
	writeJSON(w, 200, map[string]any{"runners": results})
}

func handleRegisterRunner(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	var req struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		Hostname   string `json:"hostname"`
		Platform   string `json:"platform"`
		IPAddress  string `json:"ip_address"`
		RunnerType string `json:"runner_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.RunnerType == "" {
		req.RunnerType = "customer_managed"
	}

	runnerID := "rnr_" + shortID()

	_, err := db.ExecContext(ctx,
		`INSERT INTO runners (id, organisation_id, name, runner_type, status, version, hostname, platform, ip_address, registered_at)
		VALUES (?,?,?,?,?,?,?,?,?,datetime('now'))`,
		runnerID, "org_demo", req.Name, req.RunnerType, "active",
		sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform),
		sqlNullString(req.IPAddress))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to register runner: " + err.Error()})
		return
	}

	db.ExecContext(ctx,
		"INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES (?,?,?,?,?)",
		"rsc_"+shortID(), "org_demo", runnerID, "all", "*")

	writeAuditEvent(ctx, db, "org_demo", "user_senior", "runner.registered", "runner", runnerID, "success", map[string]any{"name": req.Name})

	logger.Info("runner registered", "runner_id", runnerID, "name", req.Name)
	writeJSON(w, 201, map[string]any{"runner_id": runnerID, "status": "active"})
}

func handleRunnerHeartbeat(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	var req struct {
		RunnerID string `json:"runner_id"`
		Hostname string `json:"hostname"`
		Platform string `json:"platform"`
		Version  string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}

	_, err := db.ExecContext(ctx,
		"UPDATE runners SET status = 'active', last_seen_at = datetime('now'), hostname = COALESCE(NULLIF(?,''), hostname), platform = COALESCE(NULLIF(?,''), platform), version = COALESCE(NULLIF(?,''), version) WHERE id = ? AND organisation_id = ?",
		req.Hostname, req.Platform, req.Version, req.RunnerID, "org_demo")
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCreateRegistrationToken(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	token := shortID() + "-regtoken"
	writeJSON(w, 201, map[string]string{
		"registration_token": token,
		"expires_in":         "3600",
	})
}

func handleListExecutions(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}

	query := `SELECT e.id, e.actor_user_id, e.actor_role_at_time, e.execution_type, e.status,
		e.risk_level, e.environment, e.reason, e.command_preview, e.command_hash,
		e.timeout_seconds, e.requested_at, e.started_at, e.finished_at
		FROM executions e WHERE e.organisation_id = ?`
	args := []any{"org_demo"}
	if status != "" {
		query += " AND e.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY e.requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type execution struct {
		ID              string `json:"id"`
		ActorUserID     string `json:"actor_user_id"`
		ActorRole       string `json:"actor_role_at_time"`
		ExecutionType   string `json:"execution_type"`
		Status          string `json:"status"`
		RiskLevel       string `json:"risk_level"`
		Environment     string `json:"environment"`
		Reason          string `json:"reason"`
		CommandPreview  string `json:"command_preview"`
		CommandHash     string `json:"command_hash"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
		RequestedAt     string `json:"requested_at"`
		StartedAt       string `json:"started_at"`
		FinishedAt      string `json:"finished_at"`
		TargetCount     int    `json:"target_count"`
		SucceededCount  int    `json:"succeeded_count"`
		FailedCount     int    `json:"failed_count"`
	}
	var results []execution
	for rows.Next() {
		var e execution
		rows.Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
			&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
			&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt)
		db.QueryRowContext(ctx,
			"SELECT COUNT(*), SUM(CASE WHEN status='succeeded' THEN 1 ELSE 0 END), SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) FROM execution_targets WHERE execution_id = ?",
			e.ID).Scan(&e.TargetCount, &e.SucceededCount, &e.FailedCount)
		results = append(results, e)
	}
	writeJSON(w, 200, map[string]any{"executions": results})
}

func handleGetExecution(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, execID string) {
	var e struct {
		ID              string `json:"id"`
		ActorUserID     string `json:"actor_user_id"`
		ActorRole       string `json:"actor_role_at_time"`
		ExecutionType   string `json:"execution_type"`
		Status          string `json:"status"`
		RiskLevel       string `json:"risk_level"`
		Environment     string `json:"environment"`
		Reason          string `json:"reason"`
		CommandPreview  string `json:"command_preview"`
		CommandHash     string `json:"command_hash"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
		RequestedAt     string `json:"requested_at"`
		StartedAt       string `json:"started_at"`
		FinishedAt      string `json:"finished_at"`
		ErrorSummary    string `json:"error_summary"`
	}
	err := db.QueryRowContext(ctx,
		`SELECT id, actor_user_id, actor_role_at_time, execution_type, status,
		risk_level, environment, reason, command_preview, command_hash,
		timeout_seconds, COALESCE(requested_at,''), COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(error_summary,'')
		FROM executions WHERE id = ? AND organisation_id = ?`, execID, "org_demo",
	).Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
		&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
		&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt, &e.ErrorSummary)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "execution not found"})
		return
	}

	type targetResult struct {
		ID         string `json:"id"`
		ServerID   string `json:"server_id"`
		RunnerID   string `json:"runner_id"`
		Status     string `json:"status"`
		ExitCode   int    `json:"exit_code"`
		Stdout     string `json:"stdout"`
		Stderr     string `json:"stderr"`
		Error      string `json:"error_summary"`
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, server_id, COALESCE(runner_id,''), status, COALESCE(exit_code,0),
		'', '', COALESCE(error_summary,''),
		COALESCE(started_at,''), COALESCE(finished_at,'')
		FROM execution_targets WHERE execution_id = ? ORDER BY server_id`, execID)
	var targets []targetResult
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t targetResult
			rows.Scan(&t.ID, &t.ServerID, &t.RunnerID, &t.Status,
				&t.ExitCode, &t.Stdout, &t.Stderr, &t.Error,
				&t.StartedAt, &t.FinishedAt)
			targets = append(targets, t)
		}
	}

	writeJSON(w, 200, map[string]any{"execution": e, "targets": targets})
}

func handleCreateExecution(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
	var req struct {
		Target  string `json:"target"`
		Command string `json:"command"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Target == "" || req.Command == "" {
		writeJSON(w, 400, map[string]string{"error": "target and command are required"})
		return
	}

	targets := resolveTargets(ctx, db, req.Target)
	if len(targets) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no servers found for target: " + req.Target})
		return
	}

	execID := "exe_" + shortID()
	env := detectEnvironment(ctx, db, targets)

	_, err := db.ExecContext(ctx,
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, execution_type, status, environment, command_preview, command_hash, reason, timeout_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execID, "org_demo", "user_senior", "senior_engineer", "raw_command", "queued", env, req.Command, hashCmd(req.Command), req.Reason, 300,
	)
	if err != nil {
		logger.Error("execution create error", "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to create execution"})
		return
	}

	for _, srvID := range targets {
		db.ExecContext(ctx,
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?, ?, ?, ?, 'pending', '{}')`,
			"ext_"+shortID(), "org_demo", execID, srvID)
	}

	writeAuditEvent(ctx, db, "org_demo", "user_senior", "execution.requested", "execution", execID, "queued", map[string]any{
		"command":      req.Command,
		"reason":       req.Reason,
		"target":       req.Target,
		"target_count": len(targets),
	})

	logger.Info("execution created", "execution_id", execID, "targets", len(targets))
	writeJSON(w, 201, map[string]any{
		"execution_id": execID,
		"status":       "queued",
		"target_count": len(targets),
	})
}

func resolveTargets(ctx context.Context, db *sql.DB, target string) []string {
	if strings.HasPrefix(target, "server:") {
		serverID := target[len("server:"):]
		var exists string
		err := db.QueryRowContext(ctx, "SELECT id FROM servers WHERE (id = ? OR name = ?) AND organisation_id = ? AND status != 'archived'",
			serverID, serverID, "org_demo").Scan(&exists)
		if err != nil {
			return nil
		}
		return []string{exists}
	}
	if strings.HasPrefix(target, "tag:") {
		parts := strings.SplitN(target[len("tag:"):], "=", 2)
		if len(parts) != 2 {
			return nil
		}
		rows, err := db.QueryContext(ctx,
			"SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?", "org_demo", parts[0], parts[1])
		if err != nil {
			return nil
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			rows.Scan(&id)
			ids = append(ids, id)
		}
		return ids
	}
	// Default: try as direct server name
	var exists string
	err := db.QueryRowContext(ctx, "SELECT id FROM servers WHERE name = ? AND organisation_id = ? AND status != 'archived'",
		target, "org_demo").Scan(&exists)
	if err != nil {
		return nil
	}
	return []string{exists}
}

func detectEnvironment(ctx context.Context, db *sql.DB, serverIDs []string) string {
	if len(serverIDs) == 0 {
		return ""
	}
	var env string
	db.QueryRowContext(ctx, "SELECT environment FROM servers WHERE id = ? AND organisation_id = ?", serverIDs[0], "org_demo").Scan(&env)
	return env
}

func handleCancelExecution(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, execID string, logger *slog.Logger) {
	result, err := db.ExecContext(ctx,
		"UPDATE executions SET status = 'cancelled', finished_at = datetime('now') WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		execID, "org_demo")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to cancel"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 400, map[string]string{"error": "execution not cancellable"})
		return
	}
	db.ExecContext(ctx, "UPDATE execution_targets SET status = 'cancelled' WHERE execution_id = ?", execID)
	writeAuditEvent(ctx, db, "org_demo", "user_senior", "execution.cancelled", "execution", execID, "cancelled", nil)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func handleClaimJob(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	var execID, command string
	err := db.QueryRowContext(ctx,
		`UPDATE executions SET status = 'running', started_at = datetime('now')
		WHERE id = (SELECT id FROM executions WHERE status = 'queued' AND organisation_id = ? ORDER BY requested_at ASC LIMIT 1)
		RETURNING id, command_preview`, "org_demo",
	).Scan(&execID, &command)
	if err != nil {
		writeJSON(w, 404, map[string]string{"status": "no_jobs"})
		return
	}

	db.ExecContext(ctx,
		"UPDATE execution_targets SET status = 'running', started_at = datetime('now') WHERE execution_id = ? AND status = 'pending'", execID)

	writeJSON(w, 200, map[string]any{
		"execution_id": execID,
		"command":      command,
	})
}

func handleSubmitResult(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
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
		"UPDATE executions SET status = ?, finished_at = datetime('now'), error_summary = ? WHERE id = ?",
		status, sqlNullString(req.Error), req.ExecutionID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update execution"})
		return
	}

	db.ExecContext(ctx, "UPDATE execution_targets SET status = ?, exit_code = ?, error_summary = ?, finished_at = datetime('now') WHERE execution_id = ? AND status = 'running'",
		status, req.ExitCode, sqlNullString(req.Error), req.ExecutionID)

	writeAuditEvent(ctx, db, "org_demo", "user_senior", "execution.completed", "execution", req.ExecutionID, status, map[string]any{
		"exit_code": req.ExitCode, "duration_ms": req.DurationMs,
	})

	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleSearchAudit(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	actor := r.URL.Query().Get("actor")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}
	query := "SELECT id, organisation_id, actor_user_id, action, target_type, target_id, result, metadata, occurred_at FROM audit_events WHERE 1=1"
	var args []any
	if actor != "" {
		query += " AND actor_user_id = ?"
		args = append(args, actor)
	}
	query += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var id, orgID, actorID, action, targetType, targetID, result, metadata, createdAt string
		if err := rows.Scan(&id, &orgID, &actorID, &action, &targetType, &targetID, &result, &metadata, &createdAt); err != nil {
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
			"metadata":        metadata,
			"created_at":      createdAt,
		})
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func sqlNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func loadTags(ctx context.Context, db *sql.DB, orgID, serverID string) []tag {
	rows, err := db.QueryContext(ctx,
		"SELECT key, value FROM server_tags WHERE organisation_id = ? AND server_id = ? ORDER BY key", orgID, serverID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []tag
	for rows.Next() {
		var t tag
		if err := rows.Scan(&t.Key, &t.Value); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}
