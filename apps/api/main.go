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

	"github.com/pgd1001/svrtools/packages/authz"
	_ "modernc.org/sqlite"
)

type tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var policy = authz.NewPolicy()

type handlerFunc func(w http.ResponseWriter, r *http.Request)

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
		writeJSON(w, 200, map[string]string{"status": "ok", "version": "0.4.0"})
	})

	mux.HandleFunc("/api/v1/whoami", withAuth(db, handleWhoAmI))

	mux.HandleFunc("/api/v1/servers", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListServers(w, r)
		case http.MethodPost:
			handleAddServer(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/servers/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/servers/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing server id"})
			return
		}
		if hasSuffix(path, "/check") {
			serverID := path[:len(path)-len("/check")]
			if r.Method == http.MethodPost {
				handleCheckServer(w, r, serverID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetServer(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/runners", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListRunners(w, r)
		case http.MethodPost:
			handleRegisterRunner(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/runners/heartbeat", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleRunnerHeartbeat(w, r)
	}))

	mux.HandleFunc("/api/v1/runners/registration-token", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleCreateRegistrationToken(w, r)
	}))

	mux.HandleFunc("/api/v1/executions", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListExecutions(w, r)
		case http.MethodPost:
			handleCreateExecution(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/executions/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/executions/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing execution id"})
			return
		}
		if hasSuffix(path, "/cancel") {
			execID := path[:len(path)-len("/cancel")]
			if r.Method == http.MethodPost {
				handleCancelExecution(w, r, execID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetExecution(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		handleClaimJob(r.Context(), db, w, r)
	})

	mux.HandleFunc("/api/v1/jobs/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleSubmitResult(r.Context(), db, w, r)
	})

	mux.HandleFunc("/api/v1/audit", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		handleSearchAudit(w, r)
	}))

	mux.HandleFunc("/api/v1/runbooks", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListRunbooks(w, r)
		case http.MethodPost:
			handleCreateRunbook(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/runbooks/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/runbooks/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing runbook name"})
			return
		}
		if hasSuffix(path, "/run") {
			name := path[:len(path)-len("/run")]
			if r.Method == http.MethodPost {
				handleRunRunbook(w, r, name)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if hasSuffix(path, "/publish") {
			name := path[:len(path)-len("/publish")]
			if r.Method == http.MethodPost {
				handlePublishRunbook(w, r, name)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetRunbook(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/approvals", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListApprovals(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/approvals/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/approvals/"):]
		if hasSuffix(path, "/approve") {
			approvalID := path[:len(path)-len("/approve")]
			if r.Method == http.MethodPost {
				handleApprove(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if hasSuffix(path, "/deny") {
			approvalID := path[:len(path)-len("/deny")]
			if r.Method == http.MethodPost {
				handleDeny(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

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

type contextKey string

const dbKey contextKey = "db"

func withAuth(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-VPS-User")
		if userID == "" {
			userID = "user_senior"
		}
		actor, err := authz.ResolveDevUser(r.Context(), db, userID)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "authentication failed: " + err.Error()})
			return
		}
		ctx := authz.WithActor(r.Context(), actor)
		ctx = context.WithValue(ctx, dbKey, db)
		next(w, r.WithContext(ctx))
	}
}

func dbFrom(r *http.Request) *sql.DB {
	return r.Context().Value(dbKey).(*sql.DB)
}

func handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}
	var orgName string
	dbFrom(r).QueryRowContext(r.Context(), "SELECT name FROM organisations WHERE id = ?", actor.OrganisationID).Scan(&orgName)
	writeJSON(w, 200, map[string]any{
		"user_id":         actor.UserID,
		"email":           actor.Email,
		"name":            actor.DisplayName,
		"organisation_id": actor.OrganisationID,
		"organisation":    orgName,
		"role":            actor.Role,
	})
}

func handleListServers(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	envFilter := r.URL.Query().Get("environment")
	tagKey := r.URL.Query().Get("tag_key")
	tagValue := r.URL.Query().Get("tag_value")

	query := `SELECT s.id, s.name, s.hostname, COALESCE(s.public_ip,''), COALESCE(s.private_ip,''),
		s.ssh_port, COALESCE(s.ssh_username,''), s.environment, COALESCE(s.provider,''),
		COALESCE(s.os_name,''), COALESCE(s.os_version,''), COALESCE(s.kernel_version,''),
		COALESCE(s.architecture,''), s.status, COALESCE(s.last_seen_at,''),
		COALESCE(s.last_check_at,''), s.created_at
		FROM servers s WHERE s.organisation_id = ?`
	args := []any{actor.OrganisationID}

	if envFilter != "" {
		query += " AND s.environment = ?"
		args = append(args, envFilter)
	}
	if tagKey != "" && tagValue != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?)`
		args = append(args, actor.OrganisationID, tagKey, tagValue)
	} else if tagKey != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ?)`
		args = append(args, actor.OrganisationID, tagKey)
	}

	query += " ORDER BY s.name ASC"

	rows, err := dbFrom(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type server struct {
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
		Tags        []tag  `json:"tags"`
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
		s.Tags = loadTags(r.Context(), dbFrom(r), actor.OrganisationID, s.ID)
		servers = append(servers, s)
	}
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func handleAddServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerManagement(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.created", "server", "", dec)
		return
	}

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

	_, err := dbFrom(r).ExecContext(r.Context(),
		`INSERT INTO servers (id, organisation_id, name, hostname, public_ip, private_ip, ssh_port, ssh_username, environment, provider)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		serverID, actor.OrganisationID, req.Name, sqlNullString(req.Hostname), sqlNullString(req.PublicIP),
		sqlNullString(req.PrivateIP), req.SSHPort, sqlNullString(req.SSHUsername), req.Environment, sqlNullString(req.Provider))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create server: " + err.Error()})
		return
	}

	for _, t := range req.Tags {
		if t.Key != "" {
			dbFrom(r).ExecContext(r.Context(),
				"INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)",
				actor.OrganisationID, serverID, t.Key, t.Value)
		}
	}

	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "server.created", "server", serverID, "success", map[string]any{"name": req.Name, "environment": req.Environment})

	writeJSON(w, 201, map[string]any{"server_id": serverID, "status": "created"})
}

type serverDetail struct {
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
	Tags        []tag  `json:"tags"`
}

func handleGetServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	s := serverDetail{
		Tags: loadTags(r.Context(), db, actor.OrganisationID, serverID),
	}

	err := db.QueryRowContext(r.Context(),
		`SELECT id, name, COALESCE(hostname,''), COALESCE(public_ip,''), COALESCE(private_ip,''),
		ssh_port, COALESCE(ssh_username,''), environment, COALESCE(provider,''),
		COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''),
		COALESCE(architecture,''), status, COALESCE(last_seen_at,''),
		COALESCE(last_check_at,''), created_at
		FROM servers WHERE id = ? AND organisation_id = ?`, serverID, actor.OrganisationID,
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

func handleCheckServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerCheck(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.checked", "server", serverID, dec)
		return
	}

	db := dbFrom(r)
	var host, sshUser string
	var sshPort int
	err := db.QueryRowContext(r.Context(),
		"SELECT COALESCE(hostname, public_ip, ''), ssh_port, COALESCE(ssh_username,'') FROM servers WHERE id = ? AND organisation_id = ?",
		serverID, actor.OrganisationID).Scan(&host, &sshPort, &sshUser)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	db.ExecContext(r.Context(),
		"UPDATE servers SET status = 'active', last_check_at = ?, last_seen_at = ? WHERE id = ?",
		now, now, serverID)

	checkResult := map[string]any{
		"server_id":  serverID,
		"status":     "reachable",
		"hostname":   host,
		"ssh_port":   sshPort,
		"checked_at": now,
	}

	var osName, osVer, kernel, arch string
	db.QueryRowContext(r.Context(),
		"SELECT COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''), COALESCE(architecture,'') FROM servers WHERE id = ?",
		serverID).Scan(&osName, &osVer, &kernel, &arch)

	if osName == "" {
		osName = "linux"
		osVer = "unknown"
		kernel = "unknown"
		arch = "amd64"
		db.ExecContext(r.Context(),
			"UPDATE servers SET os_name=?, os_version=?, kernel_version=?, architecture=? WHERE id=?",
			osName, osVer, kernel, arch, serverID)
	}
	checkResult["os_name"] = osName
	checkResult["os_version"] = osVer
	checkResult["kernel_version"] = kernel
	checkResult["architecture"] = arch
	checkResult["uptime"] = "0d 0h 0m (simulated)"

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "server.checked", "server", serverID, "success", nil)
	writeJSON(w, 200, map[string]any{"server": checkResult})
}

func handleListRunners(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	rows, err := dbFrom(r).QueryContext(r.Context(),
		`SELECT id, name, runner_type, status, COALESCE(version,''), COALESCE(hostname,''),
		COALESCE(platform,''), COALESCE(ip_address,''), COALESCE(last_seen_at,''),
		COALESCE(registered_at,''), COALESCE(revoked_at,''), created_at
		FROM runners WHERE organisation_id = ? ORDER BY name ASC`, actor.OrganisationID)
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
		var rn runner
		rows.Scan(&rn.ID, &rn.Name, &rn.RunnerType, &rn.Status, &rn.Version,
			&rn.Hostname, &rn.Platform, &rn.IPAddress, &rn.LastSeenAt,
			&rn.RegisteredAt, &rn.RevokedAt, &rn.CreatedAt)
		results = append(results, rn)
	}
	writeJSON(w, 200, map[string]any{"runners": results})
}

func handleRegisterRunner(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckRunnerManagement(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "runner.registered", "runner", "", dec)
		return
	}

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

	db := dbFrom(r)
	_, err := db.ExecContext(r.Context(),
		`INSERT INTO runners (id, organisation_id, name, runner_type, status, version, hostname, platform, ip_address, registered_at)
		VALUES (?,?,?,?,?,?,?,?,?,datetime('now'))`,
		runnerID, actor.OrganisationID, req.Name, req.RunnerType, "active",
		sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform),
		sqlNullString(req.IPAddress))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to register runner: " + err.Error()})
		return
	}

	db.ExecContext(r.Context(),
		"INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES (?,?,?,?,?)",
		"rsc_"+shortID(), actor.OrganisationID, runnerID, "all", "*")

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runner.registered", "runner", runnerID, "success", map[string]any{"name": req.Name})
	writeJSON(w, 201, map[string]any{"runner_id": runnerID, "status": "active"})
}

func handleRunnerHeartbeat(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
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

	_, err := dbFrom(r).ExecContext(r.Context(),
		"UPDATE runners SET status = 'active', last_seen_at = datetime('now'), hostname = COALESCE(NULLIF(?,''), hostname), platform = COALESCE(NULLIF(?,''), platform), version = COALESCE(NULLIF(?,''), version) WHERE id = ? AND organisation_id = ?",
		req.Hostname, req.Platform, req.Version, req.RunnerID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	token := shortID() + "-regtoken"
	writeJSON(w, 201, map[string]string{
		"registration_token": token,
		"expires_in":         "3600",
	})
}

func handleListExecutions(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	status := r.URL.Query().Get("status")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}

	query := `SELECT e.id, e.actor_user_id, e.actor_role_at_time, e.execution_type, e.status,
		e.risk_level, e.environment, e.reason, e.command_preview, e.command_hash,
		e.timeout_seconds, e.requested_at, e.started_at, e.finished_at
		FROM executions e WHERE e.organisation_id = ?`
	args := []any{actor.OrganisationID}
	if status != "" {
		query += " AND e.status = ?"
		args = append(args, status)
	}
	query += " ORDER BY e.requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := dbFrom(r).QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type execution struct {
		ID             string `json:"id"`
		ActorUserID    string `json:"actor_user_id"`
		ActorRole      string `json:"actor_role_at_time"`
		ExecutionType  string `json:"execution_type"`
		Status         string `json:"status"`
		RiskLevel      string `json:"risk_level"`
		Environment    string `json:"environment"`
		Reason         string `json:"reason"`
		CommandPreview string `json:"command_preview"`
		CommandHash    string `json:"command_hash"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		RequestedAt    string `json:"requested_at"`
		StartedAt      string `json:"started_at"`
		FinishedAt     string `json:"finished_at"`
		TargetCount    int    `json:"target_count"`
		SucceededCount int    `json:"succeeded_count"`
		FailedCount    int    `json:"failed_count"`
	}
	var results []execution
	for rows.Next() {
		var e execution
		rows.Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
			&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
			&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt)
		dbFrom(r).QueryRowContext(r.Context(),
			"SELECT COUNT(*), SUM(CASE WHEN status='succeeded' THEN 1 ELSE 0 END), SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END) FROM execution_targets WHERE execution_id = ?",
			e.ID).Scan(&e.TargetCount, &e.SucceededCount, &e.FailedCount)
		results = append(results, e)
	}
	writeJSON(w, 200, map[string]any{"executions": results})
}

func handleGetExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	var e struct {
		ID             string `json:"id"`
		ActorUserID    string `json:"actor_user_id"`
		ActorRole      string `json:"actor_role_at_time"`
		ExecutionType  string `json:"execution_type"`
		Status         string `json:"status"`
		RiskLevel      string `json:"risk_level"`
		Environment    string `json:"environment"`
		Reason         string `json:"reason"`
		CommandPreview string `json:"command_preview"`
		CommandHash    string `json:"command_hash"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		RequestedAt    string `json:"requested_at"`
		StartedAt      string `json:"started_at"`
		FinishedAt     string `json:"finished_at"`
		ErrorSummary   string `json:"error_summary"`
	}
	err := db.QueryRowContext(r.Context(),
		`SELECT id, actor_user_id, actor_role_at_time, execution_type, status,
		risk_level, environment, reason, command_preview, command_hash,
		timeout_seconds, COALESCE(requested_at,''), COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(error_summary,'')
		FROM executions WHERE id = ? AND organisation_id = ?`, execID, actor.OrganisationID,
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
		StartedAt  string `json:"started_at"`
		FinishedAt string `json:"finished_at"`
		Error      string `json:"error_summary"`
	}
	rows, err := db.QueryContext(r.Context(),
		`SELECT id, server_id, COALESCE(runner_id,''), status, COALESCE(exit_code,0),
		COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(error_summary,'')
		FROM execution_targets WHERE execution_id = ? ORDER BY server_id`, execID)
	var targets []targetResult
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t targetResult
			rows.Scan(&t.ID, &t.ServerID, &t.RunnerID, &t.Status,
				&t.ExitCode, &t.StartedAt, &t.FinishedAt, &t.Error)
			targets = append(targets, t)
		}
	}

	writeJSON(w, 200, map[string]any{"execution": e, "targets": targets})
}

func handleCreateExecution(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

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

	targetIDs := resolveTargets(r.Context(), db, actor.OrganisationID, req.Target)
	if len(targetIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no servers found for target: " + req.Target})
		return
	}

	env := detectEnv(r.Context(), db, actor.OrganisationID, targetIDs)
	risk := authz.ClassifyRisk(req.Command)

	dec := policy.CheckExecution(r.Context(), db, actor, authz.Env(env), risk, req.Reason)
	if !dec.Allowed {
		writeDenial(w, r, actor, "execution.requested", "execution", req.Target, dec)
		return
	}

	execID := "exe_" + shortID()

	_, err := db.ExecContext(r.Context(),
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, execution_type, status, environment, risk_level, command_preview, command_hash, reason, timeout_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execID, actor.OrganisationID, actor.UserID, actor.Role, "raw_command", "queued", env, string(risk), req.Command, hashCmd(req.Command), req.Reason, 300,
	)
	if err != nil {
		slog.Error("execution create error", "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to create execution"})
		return
	}

	for _, srvID := range targetIDs {
		db.ExecContext(r.Context(),
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?, ?, ?, ?, 'pending', '{}')`,
			"ext_"+shortID(), actor.OrganisationID, execID, srvID)
	}

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "execution.requested", "execution", execID, "queued", map[string]any{
		"command": req.Command, "reason": req.Reason, "target": req.Target, "risk": string(risk), "target_count": len(targetIDs),
	})

	writeJSON(w, 201, map[string]any{
		"execution_id": execID,
		"status":       "queued",
		"risk_level":   string(risk),
		"target_count": len(targetIDs),
	})
}

func handleCancelExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	result, err := db.ExecContext(r.Context(),
		"UPDATE executions SET status = 'cancelled', finished_at = datetime('now') WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		execID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to cancel"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 400, map[string]string{"error": "execution not cancellable"})
		return
	}
	db.ExecContext(r.Context(), "UPDATE execution_targets SET status = 'cancelled' WHERE execution_id = ?", execID)
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "execution.cancelled", "execution", execID, "cancelled", nil)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func handleSearchAudit(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	actorFilter := r.URL.Query().Get("actor")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}
	query := "SELECT id, organisation_id, actor_user_id, action, target_type, target_id, result, metadata, occurred_at FROM audit_events WHERE organisation_id = ?"
	args := []any{actor.OrganisationID}
	if actorFilter != "" {
		query += " AND actor_user_id = ?"
		args = append(args, actorFilter)
	}
	query += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := dbFrom(r).QueryContext(r.Context(), query, args...)
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

func writeDenial(w http.ResponseWriter, r *http.Request, actor *authz.Actor, action, targetType, targetID string, dec authz.Decision) {
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, action, targetType, targetID, "denied", map[string]any{
		"reason":  dec.Reason,
		"message": dec.Message,
	})
	writeJSON(w, 403, map[string]string{
		"error":   dec.Message,
		"reason":  dec.Reason,
		"next":    "Contact your admin or request approval if available.",
	})
}

func resolveTargets(ctx context.Context, db *sql.DB, orgID, target string) []string {
	if strings.HasPrefix(target, "server:") {
		serverID := target[len("server:"):]
		var exists string
		err := db.QueryRowContext(ctx, "SELECT id FROM servers WHERE (id = ? OR name = ?) AND organisation_id = ? AND status != 'archived'",
			serverID, serverID, orgID).Scan(&exists)
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
			"SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?", orgID, parts[0], parts[1])
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
	var exists string
	err := db.QueryRowContext(ctx, "SELECT id FROM servers WHERE name = ? AND organisation_id = ? AND status != 'archived'",
		target, orgID).Scan(&exists)
	if err != nil {
		return nil
	}
	return []string{exists}
}

func detectEnv(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) string {
	if len(serverIDs) == 0 {
		return ""
	}
	var env string
	db.QueryRowContext(ctx, "SELECT environment FROM servers WHERE id = ? AND organisation_id = ?", serverIDs[0], orgID).Scan(&env)
	return env
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

func handleClaimJob(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	runnerOrg := r.URL.Query().Get("organisation_id")
	if runnerOrg == "" {
		runnerOrg = "org_demo"
	}

	var execID, command string
	err := db.QueryRowContext(ctx,
		`UPDATE executions SET status = 'running', started_at = datetime('now')
		WHERE id = (SELECT id FROM executions WHERE status = 'queued' AND organisation_id = ? ORDER BY requested_at ASC LIMIT 1)
		RETURNING id, command_preview`, runnerOrg,
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

func handleSubmitResult(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
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

	var orgID string
	db.QueryRowContext(ctx, "SELECT organisation_id FROM executions WHERE id = ?", req.ExecutionID).Scan(&orgID)
	writeAuditEvent(ctx, db, orgID, "", "execution.completed", "execution", req.ExecutionID, status, map[string]any{
		"exit_code": req.ExitCode, "duration_ms": req.DurationMs,
	})

	writeJSON(w, 200, map[string]string{"status": "ok"})
}
