package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/redact"
)

// Execution requests: creation, listing, inspection, and cancellation.

func handleListExecutions(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	status := r.URL.Query().Get("status")
	mineFilter := r.URL.Query().Get("mine")
	delegatedBy := r.URL.Query().Get("delegated_by")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}

	query := `SELECT e.id, e.actor_user_id, e.actor_role_at_time, e.execution_type, e.status,
		e.risk_level, e.environment, e.reason, e.command_preview, e.command_hash,
		e.timeout_seconds, e.requested_at, e.started_at, e.finished_at,
		COALESCE(e.delegated_by_user_id,''), COALESCE(e.approval_id,''),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id AND et.status = 'succeeded'),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id AND et.status = 'failed')
		FROM executions e WHERE e.organisation_id = ?`
	args := []any{actor.OrganisationID}
	if status != "" {
		query += " AND e.status = ?"
		args = append(args, status)
	}
	if mineFilter == "true" || mineFilter == "1" {
		query += " AND e.actor_user_id = ?"
		args = append(args, actor.UserID)
	}
	if delegatedBy != "" {
		query += " AND e.delegated_by_user_id = ?"
		args = append(args, delegatedBy)
	}
	query += " ORDER BY e.requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := apiQuery(r.Context(), readDBFrom(r), query, args...)
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
		DelegatedBy    string `json:"delegated_by_user_id"`
		ApprovalID     string `json:"approval_id"`
	}
	var results []execution
	for rows.Next() {
		var e execution
		rows.Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
			&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
			&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt,
			&e.DelegatedBy, &e.ApprovalID, &e.TargetCount, &e.SucceededCount, &e.FailedCount)
		results = append(results, e)
	}
	writeJSON(w, 200, map[string]any{"executions": results})
}

func handleGetExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := readDBFrom(r)
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
		DelegatedBy    string `json:"delegated_by_user_id"`
		ApprovalID     string `json:"approval_id"`
	}
	err := apiQueryRow(r.Context(), db,
		`SELECT id, actor_user_id, actor_role_at_time, execution_type, status,
		risk_level, environment, reason, command_preview, command_hash,
		timeout_seconds, COALESCE(requested_at,''), COALESCE(started_at,''), COALESCE(finished_at,''),
		COALESCE(error_summary,''), COALESCE(delegated_by_user_id,''), COALESCE(approval_id,'')
		FROM executions WHERE id = ? AND organisation_id = ?`, execID, actor.OrganisationID,
	).Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
		&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
		&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt, &e.ErrorSummary,
		&e.DelegatedBy, &e.ApprovalID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "execution not found"})
		return
	}

	type targetResult struct {
		ID               string `json:"id"`
		ServerID         string `json:"server_id"`
		RunnerID         string `json:"runner_id"`
		Status           string `json:"status"`
		ExitCode         int    `json:"exit_code"`
		Stdout           string `json:"stdout"`
		Stderr           string `json:"stderr"`
		StartedAt        string `json:"started_at"`
		FinishedAt       string `json:"finished_at"`
		Error            string `json:"error_summary"`
		StdoutArtifactID string `json:"-"`
		StderrArtifactID string `json:"-"`
	}
	rows, err := apiQuery(r.Context(), db,
		`SELECT id, server_id, COALESCE(runner_id,''), status, COALESCE(exit_code,0),
		stdout, stderr, COALESCE(stdout_artifact_id,''), COALESCE(stderr_artifact_id,''), COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(error_summary,'')
		FROM execution_targets WHERE execution_id = ? ORDER BY server_id`, execID)
	var targets []targetResult
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t targetResult
			rows.Scan(&t.ID, &t.ServerID, &t.RunnerID, &t.Status,
				&t.ExitCode, &t.Stdout, &t.Stderr, &t.StdoutArtifactID, &t.StderrArtifactID, &t.StartedAt, &t.FinishedAt, &t.Error)
			if t.StdoutArtifactID != "" && apiArtifacts != nil {
				if data, _, artifactErr := apiArtifacts.Get(t.StdoutArtifactID); artifactErr == nil {
					t.Stdout = string(data)
				}
			}
			if t.StderrArtifactID != "" && apiArtifacts != nil {
				if data, _, artifactErr := apiArtifacts.Get(t.StderrArtifactID); artifactErr == nil {
					t.Stderr = string(data)
				}
			}
			targets = append(targets, t)
		}
	}
	type executionEvent struct {
		ID         string `json:"id"`
		TargetID   string `json:"target_id"`
		FromStatus string `json:"from_status"`
		ToStatus   string `json:"to_status"`
		EventType  string `json:"event_type"`
		Metadata   string `json:"metadata"`
		OccurredAt string `json:"occurred_at"`
	}
	eventRows, err := apiQuery(r.Context(), db, `SELECT id, COALESCE(target_id,''), COALESCE(from_status,''), to_status, event_type, metadata, occurred_at FROM execution_events WHERE execution_id = ? AND organisation_id = ? ORDER BY occurred_at ASC, id ASC`, execID, actor.OrganisationID)
	var events []executionEvent
	if err == nil {
		defer eventRows.Close()
		for eventRows.Next() {
			var event executionEvent
			if eventRows.Scan(&event.ID, &event.TargetID, &event.FromStatus, &event.ToStatus, &event.EventType, &event.Metadata, &event.OccurredAt) == nil {
				events = append(events, event)
			}
		}
	}

	writeJSON(w, 200, map[string]any{"execution": e, "targets": targets, "events": events})
}

func handleCreateExecution(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var req struct {
		Target      string `json:"target"`
		Command     string `json:"command"`
		Reason      string `json:"reason"`
		DelegatedBy string `json:"delegated_by_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Target == "" || req.Command == "" {
		writeJSON(w, 400, map[string]string{"error": "target and command are required"})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey != "" && !validIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key must contain only letters, numbers, '.', '_' or '-' and be at most 128 characters"})
		return
	}
	payloadHash := hashPayload(req)
	if idempotencyKey != "" {
		if replayed, err := replayIdempotentExecution(r.Context(), db, actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, w); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect idempotency key"})
			return
		} else if replayed {
			return
		}
	}

	targetIDs := resolveTargets(r.Context(), db, actor.OrganisationID, req.Target)
	if len(targetIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no servers found for target: " + req.Target})
		return
	}

	env, mixed := targetEnvironment(r.Context(), db, actor.OrganisationID, targetIDs)
	if mixed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "targets span multiple environments; submit one environment at a time"})
		return
	}
	risk := authz.ClassifyRisk(req.Command)

	dec := policy.CheckExecution(r.Context(), db, actor, authz.Env(env), risk, req.Reason)
	if !dec.Allowed {
		writeDenial(w, r, actor, "execution.requested", "execution", req.Target, dec)
		return
	}

	execID := "exe_" + shortID()

	targetSnapshot := snapshotTargets(r.Context(), db, actor.OrganisationID, targetIDs)
	if len(targetSnapshot) != len(targetIDs) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to snapshot selected targets"})
		return
	}
	tx, err := beginAPITx(r.Context(), db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start execution transaction"})
		return
	}
	defer tx.Rollback()
	_, err = apiExec(r.Context(), tx,
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, delegated_by_user_id, execution_type, status, environment, risk_level, command, command_preview, command_hash, reason, timeout_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execID, actor.OrganisationID, actor.UserID, actor.Role, sqlNullString(req.DelegatedBy), "raw_command", "queued", env, string(risk), req.Command, redact.Stdout(req.Command), hashCmd(req.Command), req.Reason, 300,
	)
	if err != nil {
		slog.Error("execution create error", "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to create execution"})
		return
	}

	for i, srvID := range targetIDs {
		if _, err = apiExec(r.Context(), tx,
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?, ?, ?, ?, 'pending', `+metadataRuntime().JSONParameter()+`)`,
			"ext_"+shortID(), actor.OrganisationID, execID, srvID, jsonString(targetSnapshot[i])); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create execution target"})
			return
		}
	}
	if err = recordExecutionEvent(r.Context(), tx, actor.OrganisationID, execID, "", "", "queued", "execution.queued", map[string]any{"execution_type": "raw_command"}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if err = writeAuditEventTx(r.Context(), tx, actor.OrganisationID, actor.UserID, "execution.requested", "execution", execID, "queued", map[string]any{
		"command": redact.Stdout(req.Command), "reason": req.Reason, "target": req.Target, "risk": string(risk), "target_count": len(targetIDs),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution audit event"})
		return
	}
	responseBody := jsonString(map[string]any{
		"execution_id": execID,
		"status":       "queued",
		"risk_level":   string(risk),
		"target_count": len(targetIDs),
	})
	if idempotencyKey != "" {
		if _, err = apiExec(r.Context(), tx,
			`INSERT INTO execution_idempotency (organisation_id, user_id, idempotency_key, payload_hash, execution_id, response_body) VALUES (?,?,?,?,?,?)`,
			actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, execID, responseBody); err != nil {
			_ = tx.Rollback()
			if replayed, replayErr := replayIdempotentExecution(r.Context(), db, actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, w); replayErr == nil && replayed {
				return
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key is already in use"})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit execution"})
		return
	}
	if err := publishPendingJobNotifications(r.Context(), db, execID); err != nil {
		slog.Warn("execution committed but JetStream notification publication was incomplete", "execution_id", execID, "error", err)
	}

	writeRawJSON(w, http.StatusCreated, responseBody)
}

func replayIdempotentExecution(ctx context.Context, db *sql.DB, orgID, userID, key, payloadHash string, w http.ResponseWriter) (bool, error) {
	var storedHash, responseBody string
	err := apiQueryRow(ctx, db,
		`SELECT payload_hash, response_body FROM execution_idempotency WHERE organisation_id = ? AND user_id = ? AND idempotency_key = ?`,
		orgID, userID, key).Scan(&storedHash, &responseBody)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedHash != payloadHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key was already used with a different request payload"})
		return true, nil
	}
	w.Header().Set("Idempotency-Replayed", "true")
	writeRawJSON(w, http.StatusCreated, responseBody)
	return true, nil
}

func handleCancelExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	var requesterID string
	if err := apiQueryRow(r.Context(), db,
		"SELECT actor_user_id FROM executions WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		execID, actor.OrganisationID).Scan(&requesterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "execution not cancellable"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution"})
		return
	}
	if actor.UserID != requesterID && !canManageExecution(actor.Role) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the requester or a senior operator can cancel this execution"})
		return
	}
	result, err := apiExec(r.Context(), db,
		"UPDATE executions SET status = 'cancelled', finished_at = ? WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		time.Now().UTC(), execID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to cancel"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 400, map[string]string{"error": "execution not cancellable"})
		return
	}
	_, _ = apiExec(r.Context(), db, "UPDATE execution_targets SET status = 'cancelled' WHERE execution_id = ?", execID)
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "execution.cancelled", "execution", execID, "cancelled", nil)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func canManageExecution(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "owner", "admin", "senior_engineer":
		return true
	default:
		return false
	}
}

func resolveTargets(ctx context.Context, db *sql.DB, orgID, target string) []string {
	if strings.HasPrefix(target, "server:") {
		serverID := target[len("server:"):]
		var exists string
		err := apiQueryRow(ctx, db, "SELECT id FROM servers WHERE (id = ? OR name = ?) AND organisation_id = ? AND status != 'archived'",
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
		rows, err := apiQuery(ctx, db,
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
	err := apiQueryRow(ctx, db, "SELECT id FROM servers WHERE name = ? AND organisation_id = ? AND status != 'archived'",
		target, orgID).Scan(&exists)
	if err != nil {
		return nil
	}
	return []string{exists}
}

func detectEnv(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) string {
	env, _ := targetEnvironment(ctx, db, orgID, serverIDs)
	return env
}

func targetEnvironment(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) (string, bool) {
	if len(serverIDs) == 0 {
		return "", false
	}
	environments := make(map[string]struct{})
	for _, serverID := range serverIDs {
		var env string
		if err := apiQueryRow(ctx, db, "SELECT environment FROM servers WHERE id = ? AND organisation_id = ?", serverID, orgID).Scan(&env); err != nil {
			return "", true
		}
		environments[env] = struct{}{}
	}
	if len(environments) != 1 {
		return "", true
	}
	for env := range environments {
		return env, false
	}
	return "", true
}

func snapshotTargets(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) []map[string]any {
	snapshots := make([]map[string]any, 0, len(serverIDs))
	for _, serverID := range serverIDs {
		var id, name, hostname, environment, status string
		var port int
		if err := apiQueryRow(ctx, db,
			"SELECT id, name, COALESCE(hostname,''), environment, status, ssh_port FROM servers WHERE id = ? AND organisation_id = ?",
			serverID, orgID).Scan(&id, &name, &hostname, &environment, &status, &port); err != nil {
			return nil
		}
		snapshots = append(snapshots, map[string]any{
			"id": id, "name": name, "hostname": hostname, "environment": environment,
			"status": status, "ssh_port": port,
		})
	}
	return snapshots
}
