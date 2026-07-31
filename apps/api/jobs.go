package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pgd1001/svrtools/packages/jobsign"
	"github.com/pgd1001/svrtools/packages/redact"
)

// The runner-facing job queue: claiming work, renewing leases, and recording
// results.
//
// These are the only endpoints a runner calls during normal operation, and they
// are the enforcement point for the runner trust boundary: work is dispatched
// with a signature and accepted back only from the runner that holds the lease.

func handleClaimJob(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	runtime := metadataRuntime()
	runnerOrg, err := authenticateBoundRunner(db, r, r.URL.Query().Get("runner_id"))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, runnerOrg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	runnerID := r.URL.Query().Get("runner_id")
	if runnerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_id is required"})
		return
	}
	targetFilter := r.URL.Query().Get("target_id")
	tx, err := beginAPITx(ctx, db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start job claim"})
		return
	}
	defer tx.Rollback()
	// Expired leases are reclaimed by the claim query below regardless, so
	// reconciliation only has to run often enough to apply backoff and
	// dead-lettering. Throttling it keeps a fleet of polling runners from
	// turning every claim into a full scan.
	if claimReconcileThrottle.due(runnerOrg, time.Now()) {
		if err := reconcileExpiredLeases(ctx, tx, runnerOrg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reconcile expired jobs"})
			return
		}
	}
	var targetID, execID, command, host, sshUser, sourceStatus string
	var sshPort, timeout int
	err = runtime.QueryRowContext(ctx, tx, `SELECT et.id, e.id, et.status, COALESCE(NULLIF(e.command,''), e.command_preview), COALESCE(s.hostname, s.public_ip, ''), s.ssh_port, COALESCE(s.ssh_username,''), e.timeout_seconds
		FROM execution_targets et JOIN executions e ON e.id = et.execution_id JOIN servers s ON s.id = et.server_id
		WHERE e.status IN ('queued','running') AND e.organisation_id = ?
		AND (? = '' OR et.id = ?)
		AND ((et.status = 'pending' AND (et.next_attempt_at IS NULL OR et.next_attempt_at <= `+runtime.CurrentTime()+`))
			OR (et.status = 'running' AND et.lease_expires_at IS NOT NULL AND et.lease_expires_at <= `+runtime.CurrentTime()+`))
		AND et.attempt < et.max_attempts
		AND EXISTS (SELECT 1 FROM runners rn JOIN runner_scopes rs ON rs.runner_id = rn.id
			WHERE rn.id = ? AND rn.organisation_id = ? AND rn.status = 'active'
			AND (rs.scope_type = 'all' OR (rs.scope_type = 'server' AND rs.scope_value = et.server_id)))
		ORDER BY e.requested_at ASC LIMIT 1`, runnerOrg, targetFilter, targetFilter, runnerID, runnerOrg).Scan(&targetID, &execID, &sourceStatus, &command, &host, &sshPort, &sshUser, &timeout)
	if err != nil {
		if err := tx.Commit(); err != nil {
			_ = err
		}
		writeJSON(w, 404, map[string]string{"status": "no_jobs"})
		return
	}
	leaseID := "lease_" + shortID()
	leaseExpires := time.Now().UTC().Add(5 * time.Minute)
	claimResult, err := runtime.ExecContext(ctx, tx, "UPDATE execution_targets SET status = 'running', runner_id = ?, lease_id = ?, lease_expires_at = ?, attempt = attempt + 1, started_at = COALESCE(started_at, "+runtime.CurrentTime()+") WHERE id = ? AND status = ? AND attempt < max_attempts AND ((status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= "+runtime.CurrentTime()+")) OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= "+runtime.CurrentTime()+"))", runnerID, leaseID, leaseExpires, targetID, sourceStatus)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to claim job"})
		return
	}
	if affected, _ := claimResult.RowsAffected(); affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job was claimed by another runner"})
		return
	}
	if _, err = runtime.ExecContext(ctx, tx, "UPDATE executions SET status = 'running', started_at = COALESCE(started_at, "+runtime.CurrentTime()+") WHERE id = ? AND status IN ('queued','running')", execID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job was claimed by another runner"})
		return
	}
	if err = recordExecutionEvent(ctx, tx, runnerOrg, execID, targetID, sourceStatus, "running", "execution.started", map[string]any{"runner_id": runnerID, "lease_id": leaseID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit job claim"})
		return
	}

	// Authenticate the dispatched job. The runner performs no authorisation of
	// its own, so the signature is what proves the control plane approved this
	// exact command against this exact host for this lease.
	claims := jobsign.Claims{
		ExecutionID:   execID,
		TargetID:      targetID,
		LeaseID:       leaseID,
		RunnerID:      runnerID,
		Command:       command,
		Host:          host,
		Port:          sshPort,
		User:          sshUser,
		Timeout:       timeout,
		ExpiresAtUnix: leaseExpires.Unix(),
	}
	signature, err := apiJobSigner.Sign(claims)
	if err != nil {
		// The work is already claimed, but dispatching an unsigned job would
		// break the trust boundary. Fail the request and let the lease expire
		// back into the queue.
		slog.Error("job claim signing failed", "execution_id", execID, "target_id", targetID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to sign job"})
		return
	}
	writeJSON(w, 200, map[string]any{
		"target_id":       targetID,
		"execution_id":    execID,
		"command":         command,
		"host":            host,
		"port":            sshPort,
		"user":            sshUser,
		"timeout":         timeout,
		"lease_id":        leaseID,
		"runner_id":       runnerID,
		"expires_at_unix": leaseExpires.Unix(),
		"signature":       signature,
	})
}

func handleSubmitResult(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// Resolve the credential before the body is read so the tenant connection
	// can be bound. The runner named in the body is checked against this same
	// credential below, once it has been decoded.
	orgID, credentialRunnerID, err := authenticateRunnerCredential(db, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if credentialRunnerID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "this endpoint requires a runner-bound credential; register the runner first"})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
		ExitCode    int    `json:"exit_code"`
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		Error       string `json:"error"`
		DurationMs  int64  `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExecutionID == "" || req.TargetID == "" || req.RunnerID == "" || req.LeaseID == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if _, err := authenticateBoundRunner(db, r, req.RunnerID); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	req.Stdout = redact.Stdout(req.Stdout)
	req.Stderr = redact.Stdout(req.Stderr)
	status := "succeeded"
	if req.ExitCode != 0 || req.Error != "" {
		status = "failed"
	}
	payloadBytes, _ := json.Marshal(struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
		ExitCode    int    `json:"exit_code"`
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		Error       string `json:"error"`
		DurationMs  int64  `json:"duration_ms"`
	}{req.ExecutionID, req.TargetID, req.RunnerID, req.LeaseID, req.ExitCode, req.Stdout, req.Stderr, req.Error, req.DurationMs})
	hash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	tx, err := beginAPITx(ctx, db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start result transaction"})
		return
	}
	defer tx.Rollback()
	var receiptHash, receiptBody string
	var receiptCode int
	if err := apiQueryRow(ctx, tx, `SELECT payload_hash, response_code, response_body
		FROM execution_result_receipts WHERE target_id = ? AND lease_id = ?`, req.TargetID, req.LeaseID).
		Scan(&receiptHash, &receiptCode, &receiptBody); err == nil {
		if receiptHash != hash {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "result payload conflicts with existing receipt"})
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to replay result receipt"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(receiptCode)
		_, _ = w.Write([]byte(receiptBody))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect result receipt"})
		return
	}

	var attempt, maxAttempts int
	var currentStatus string
	if err := apiQueryRow(ctx, tx, `SELECT status, attempt, max_attempts FROM execution_targets
		WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running'`,
		req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID).Scan(&currentStatus, &attempt, &maxAttempts); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "target is not assigned to this runner"})
		return
	}
	stdoutArtifactID, stderrArtifactID, err := persistExecutionArtifacts(ctx, tx, orgID, req.ExecutionID, req.TargetID, req.Stdout, req.Stderr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist execution output"})
		return
	}
	storedStdout, storedStderr := req.Stdout, req.Stderr
	if stdoutArtifactID != "" {
		storedStdout = ""
	}
	if stderrArtifactID != "" {
		storedStderr = ""
	}
	if status == "failed" && attempt >= maxAttempts {
		status = "dead_letter"
	}

	targetResult, err := apiExec(ctx, tx, "UPDATE execution_targets SET status = ?, exit_code = ?, error_summary = ?, stdout = ?, stderr = ?, stdout_artifact_id = ?, stderr_artifact_id = ?, stdout_bytes = ?, stderr_bytes = ?, lease_expires_at = NULL, finished_at = ? WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running' AND EXISTS (SELECT 1 FROM executions e WHERE e.id = execution_targets.execution_id AND e.organisation_id = ?) AND EXISTS (SELECT 1 FROM runners rn WHERE rn.id = execution_targets.runner_id AND rn.organisation_id = ?)",
		status, req.ExitCode, sqlNullString(req.Error), storedStdout, storedStderr, sqlNullString(stdoutArtifactID), sqlNullString(stderrArtifactID), len(req.Stdout), len(req.Stderr), time.Now().UTC(), req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID, orgID, orgID)
	if err != nil || func() bool { n, _ := targetResult.RowsAffected(); return n == 0 }() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "target is not assigned to this runner"})
		return
	}
	if status == "dead_letter" {
		if _, err := apiExec(ctx, tx, "UPDATE execution_targets SET error_summary = COALESCE(NULLIF(error_summary,''), 'maximum attempts exhausted') WHERE id = ?", req.TargetID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark dead-letter job"})
			return
		}
	}
	if status == "failed" && attempt < maxAttempts {
		backoff := retryBackoffSeconds(attempt)
		retryResult, err := apiExec(ctx, tx, `UPDATE execution_targets
			SET status = 'pending', runner_id = NULL, lease_id = NULL,
			    lease_expires_at = NULL, finished_at = NULL,
			    next_attempt_at = ?
			WHERE id = ? AND status = 'failed'`, time.Now().UTC().Add(time.Duration(backoff)*time.Second), req.TargetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to schedule retry"})
			return
		}
		if affected, _ := retryResult.RowsAffected(); affected != 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "target retry state changed"})
			return
		}
		if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, req.TargetID, "failed", "pending", "execution.target.retry_scheduled", map[string]any{
			"retry_at_seconds": backoff, "attempt": attempt, "max_attempts": maxAttempts,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record retry timeline"})
			return
		}
		if err := writeAuditEventTx(ctx, tx, orgID, "", "execution.retry_scheduled", "execution", req.ExecutionID, "pending", map[string]any{
			"target_id": req.TargetID, "attempt": attempt, "max_attempts": maxAttempts,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record retry audit"})
			return
		}
		responseBody := `{"status":"retry_scheduled"}`
		if err := insertResultReceipt(ctx, tx, orgID, req, hash, responseBody); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record result receipt"})
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit result"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "retry_scheduled"})
		return
	}
	var remaining, failed int
	if err := apiQueryRow(ctx, tx, "SELECT COUNT(*) FROM execution_targets WHERE execution_id = ? AND status IN ('pending','running')", req.ExecutionID).Scan(&remaining); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution state"})
		return
	}
	if err := apiQueryRow(ctx, tx, "SELECT COUNT(*) FROM execution_targets WHERE execution_id = ? AND status IN ('failed','dead_letter')", req.ExecutionID).Scan(&failed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution result"})
		return
	}
	if remaining == 0 {
		finalStatus := "succeeded"
		if failed > 0 {
			finalStatus = "failed"
		}
		finalizeResult, err := apiExec(ctx, tx, "UPDATE executions SET status = ?, finished_at = ?, error_summary = ? WHERE id = ? AND organisation_id = ? AND status = 'running'", finalStatus, time.Now().UTC(), sqlNullString(req.Error), req.ExecutionID, orgID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to finalize execution"})
			return
		}
		if affected, _ := finalizeResult.RowsAffected(); affected != 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "execution is no longer running"})
			return
		}
		status = finalStatus
	}
	if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, req.TargetID, currentStatus, status, "execution.target.completed", map[string]any{"exit_code": req.ExitCode, "duration_ms": req.DurationMs}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if remaining == 0 {
		if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, "", "running", status, "execution.completed", map[string]any{"failed_targets": failed}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
			return
		}
	}
	action := "execution.target.completed"
	if remaining == 0 {
		action = "execution.completed"
	}
	if err := writeAuditEventTx(ctx, tx, orgID, "", action, "execution", req.ExecutionID, status, map[string]any{
		"exit_code": req.ExitCode, "duration_ms": req.DurationMs,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution audit"})
		return
	}
	responseBody := `{"status":"ok"}`
	if err := insertResultReceipt(ctx, tx, orgID, req, hash, responseBody); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record result receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit result"})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleRenewLease(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExecutionID == "" || req.TargetID == "" || req.RunnerID == "" || req.LeaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lease renewal body"})
		return
	}
	orgID, err := authenticateBoundRunner(db, r, req.RunnerID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	leaseExpires := time.Now().UTC().Add(5 * time.Minute)
	claimed, err := apiCheckedExec(ctx, db, `UPDATE execution_targets
		SET lease_expires_at = ?
		WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running'`,
		leaseExpires, req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to renew lease"})
		return
	}
	if !claimed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "lease is no longer active"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renewed"})
}

func insertResultReceipt(ctx context.Context, tx *sql.Tx, orgID string, req struct {
	ExecutionID string `json:"execution_id"`
	TargetID    string `json:"target_id"`
	RunnerID    string `json:"runner_id"`
	LeaseID     string `json:"lease_id"`
	ExitCode    int    `json:"exit_code"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	Error       string `json:"error"`
	DurationMs  int64  `json:"duration_ms"`
}, payloadHash, responseBody string) error {
	_, err := apiExec(ctx, tx, `INSERT INTO execution_result_receipts
		(organisation_id, execution_id, target_id, lease_id, runner_id, payload_hash, response_code, response_body)
		VALUES (?, ?, ?, ?, ?, ?, 200, ?)`, orgID, req.ExecutionID, req.TargetID, req.LeaseID, req.RunnerID, payloadHash, responseBody)
	return err
}

const artifactInlineLimit = 64 * 1024

func persistExecutionArtifacts(ctx context.Context, db auditExec, orgID, executionID, targetID, stdout, stderr string) (string, string, error) {
	if apiArtifacts == nil {
		return "", "", nil
	}
	var stdoutID, stderrID string
	store := func(kind, value string) (string, error) {
		if len(value) <= artifactInlineLimit {
			return "", nil
		}
		id := fmt.Sprintf("art_%s_%s_%s", executionID, targetID, kind)
		meta, err := apiArtifacts.Put(id, "text/plain", []byte(value))
		if err != nil {
			return "", err
		}
		backend := apiBackends.ArtifactStore
		if backend == "" {
			backend = "local"
		}
		if _, err := apiExec(ctx, db, `INSERT INTO artifact_records (id, organisation_id, owner_type, owner_id, content_type, byte_size, sha256, backend)
			VALUES (?,?,?,?,?,?,?,?)
			ON CONFLICT (id) DO UPDATE SET organisation_id = excluded.organisation_id,
			owner_type = excluded.owner_type, owner_id = excluded.owner_id,
			content_type = excluded.content_type, byte_size = excluded.byte_size,
			sha256 = excluded.sha256, backend = excluded.backend`, id, orgID, "execution_target_"+kind, targetID, meta.ContentType, meta.Size, meta.SHA256, backend); err != nil {
			return "", err
		}
		return id, nil
	}
	var err error
	if stdoutID, err = store("stdout", stdout); err != nil {
		return "", "", err
	}
	if stderrID, err = store("stderr", stderr); err != nil {
		return "", "", err
	}
	return stdoutID, stderrID, nil
}
