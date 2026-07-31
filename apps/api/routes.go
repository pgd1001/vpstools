package main

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

// Routing is declared here in one table rather than spread across nested
// method switches and manual path slicing.
//
// Go 1.22 ServeMux matches on method and path pattern, so "POST
// /api/v1/servers/{id}/check" replaces a chain of suffix tests and index
// arithmetic. Unmatched methods on a known path produce 405 from the mux
// itself, so handlers no longer repeat that check.
//
// Handlers that need a path segment read it with r.PathValue.

// pathHandler adapts a handler that takes a path segment to an http.HandlerFunc.
func pathHandler(name string, next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := r.PathValue(name)
		if value == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing " + name})
			return
		}
		next(w, r, value)
	}
}

// registerRoutes wires every endpoint. Authenticated routes are wrapped in
// withAuth; the runner job endpoints authenticate against runner credentials
// inside the handler instead, and registration/heartbeat are reachable before
// a user session exists.
func registerRoutes(mux *http.ServeMux, db *sql.DB, artifactMetricsDir string) {
	// Operational endpoints.
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "ok", "database": "ok", "version": version,
			"deployment_tier": apiBackends.Tier(), "database_driver": apiBackends.DatabaseDriver,
			"artifact_store": apiBackends.ArtifactStore, "job_dispatch": apiBackends.JobDispatch,
		})
	})
	mux.HandleFunc("GET /api/v1/ready", func(w http.ResponseWriter, r *http.Request) {
		metrics.readinessChecks.Add(1)
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			metrics.readinessFailed.Add(1)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "database": "unavailable"})
			return
		}
		if apiArtifacts == nil || apiArtifacts.Check() != nil {
			metrics.readinessFailed.Add(1)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "database": "ok", "artifacts": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "database": "ok", "artifacts": "ok"})
	})
	mux.HandleFunc("GET /metrics", metricsHandlerWithDB(db, artifactMetricsDir))

	// Identity and API tokens.
	mux.HandleFunc("GET /api/v1/whoami", withAuth(db, handleWhoAmI))
	mux.HandleFunc("POST /api/v1/ai/analyze", withAuth(db, handleAIAnalyze))
	mux.HandleFunc("POST /api/v1/auth/tokens", withAuth(db, handleCreateAPIToken))
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{tokenID}", withAuth(db, pathHandler("tokenID", handleRevokeAPIToken)))

	// Server inventory.
	mux.HandleFunc("GET /api/v1/servers", withAuth(db, handleListServers))
	mux.HandleFunc("POST /api/v1/servers", withAuth(db, handleAddServer))
	mux.HandleFunc("GET /api/v1/servers/{serverID}", withAuth(db, pathHandler("serverID", handleGetServer)))
	mux.HandleFunc("PUT /api/v1/servers/{serverID}", withAuth(db, pathHandler("serverID", handleUpdateServer)))
	mux.HandleFunc("PATCH /api/v1/servers/{serverID}", withAuth(db, pathHandler("serverID", handleUpdateServer)))
	mux.HandleFunc("DELETE /api/v1/servers/{serverID}", withAuth(db, pathHandler("serverID", handleArchiveServer)))
	mux.HandleFunc("POST /api/v1/servers/{serverID}/check", withAuth(db, pathHandler("serverID", handleCheckServer)))

	// Runner lifecycle. Registration and heartbeat authenticate with runner
	// credentials rather than a user session.
	mux.HandleFunc("GET /api/v1/runners", withAuth(db, handleListRunners))
	mux.HandleFunc("POST /api/v1/runners", handleRegisterRunner)
	mux.HandleFunc("POST /api/v1/runners/heartbeat", handleRunnerHeartbeat)
	mux.HandleFunc("POST /api/v1/runners/registration-token", withAuth(db, handleCreateRegistrationToken))
	mux.HandleFunc("POST /api/v1/runners/manage", withAuth(db, handleCreateManagedRunner))
	mux.HandleFunc("PUT /api/v1/runners/{runnerID}", withAuth(db, pathHandler("runnerID", handleUpdateRunner)))
	mux.HandleFunc("PATCH /api/v1/runners/{runnerID}", withAuth(db, pathHandler("runnerID", handleUpdateRunner)))
	mux.HandleFunc("DELETE /api/v1/runners/{runnerID}", withAuth(db, pathHandler("runnerID", handleRevokeRunner)))
	mux.HandleFunc("POST /api/v1/runners/{runnerID}/rotate-token", withAuth(db, pathHandler("runnerID", handleRotateRunnerToken)))

	// Executions.
	mux.HandleFunc("GET /api/v1/executions", withAuth(db, handleListExecutions))
	mux.HandleFunc("POST /api/v1/executions", withAuth(db, handleCreateExecution))
	mux.HandleFunc("GET /api/v1/executions/{execID}", withAuth(db, pathHandler("execID", handleGetExecution)))
	mux.HandleFunc("POST /api/v1/executions/{execID}/cancel", withAuth(db, pathHandler("execID", handleCancelExecution)))

	// Runner-facing job queue. These authenticate against runner credentials
	// inside the handler, which is why they are not wrapped in withAuth.
	mux.HandleFunc("GET /api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		handleClaimJob(r.Context(), db, w, r)
	})
	mux.HandleFunc("POST /api/v1/jobs/result", func(w http.ResponseWriter, r *http.Request) {
		handleSubmitResult(r.Context(), db, w, r)
	})
	mux.HandleFunc("POST /api/v1/jobs/renew", func(w http.ResponseWriter, r *http.Request) {
		handleRenewLease(r.Context(), db, w, r)
	})

	// Audit.
	mux.HandleFunc("GET /api/v1/audit", withAuth(db, handleSearchAudit))
	mux.HandleFunc("GET /api/v1/audit/verify", withAuth(db, handleVerifyAudit))

	// Runbooks.
	mux.HandleFunc("GET /api/v1/runbooks", withAuth(db, handleListRunbooks))
	mux.HandleFunc("POST /api/v1/runbooks", withAuth(db, handleCreateRunbook))
	mux.HandleFunc("GET /api/v1/runbooks/{name}", withAuth(db, pathHandler("name", handleGetRunbook)))
	mux.HandleFunc("PUT /api/v1/runbooks/{name}", withAuth(db, pathHandler("name", handleUpdateRunbook)))
	mux.HandleFunc("PATCH /api/v1/runbooks/{name}", withAuth(db, pathHandler("name", handleUpdateRunbook)))
	mux.HandleFunc("DELETE /api/v1/runbooks/{name}", withAuth(db, pathHandler("name", handleArchiveRunbook)))
	mux.HandleFunc("POST /api/v1/runbooks/{name}/run", withAuth(db, pathHandler("name", handleRunRunbook)))
	mux.HandleFunc("POST /api/v1/runbooks/{name}/publish", withAuth(db, pathHandler("name", handlePublishRunbook)))

	// Approvals.
	mux.HandleFunc("GET /api/v1/approvals", withAuth(db, handleListApprovals))
	mux.HandleFunc("GET /api/v1/approvals/{approvalID}", withAuth(db, pathHandler("approvalID", handleGetApproval)))
	mux.HandleFunc("POST /api/v1/approvals/{approvalID}/approve", withAuth(db, pathHandler("approvalID", handleApprove)))
	mux.HandleFunc("POST /api/v1/approvals/{approvalID}/deny", withAuth(db, pathHandler("approvalID", handleDeny)))

	// Automation and schedules.
	mux.HandleFunc("GET /api/v1/schedules", withAuth(db, handleListSchedules))
	mux.HandleFunc("POST /api/v1/schedules", withAuth(db, handleCreateSchedule))
	mux.HandleFunc("DELETE /api/v1/schedules/{scheduleID}", withAuth(db, pathHandler("scheduleID", handleDisableSchedule)))
	mux.HandleFunc("GET /api/v1/automation/status", withAuth(db, handleAutomationStatus))
	mux.HandleFunc("POST /api/v1/automation/pause", withAuth(db, handlePauseAutomation))
	mux.HandleFunc("POST /api/v1/automation/resume", withAuth(db, handleResumeAutomation))
}
