package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/authz"
)

// Runner lifecycle: registration, heartbeat, scoping, and credential issuance.
//
// Runners are the only component that executes anything, so their identity and
// credential handling is deliberately kept together and separate from the job
// queue that dispatches work to them.

func handleListRunners(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	rows, err := apiQuery(r.Context(), readDBFrom(r),
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

func handleCreateManagedRunner(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.created", "runner", "", authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	var req struct {
		Name       string `json:"name"`
		RunnerType string `json:"runner_type"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.RunnerType == "" {
		req.RunnerType = "customer_managed"
	}
	id := "rnr_" + shortID()
	_, err := apiExec(r.Context(), dbFrom(r), `INSERT INTO runners (id, organisation_id, name, runner_type, status) VALUES (?,?,?,?, 'pending')`, id, actor.OrganisationID, req.Name, req.RunnerType)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create runner"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.created", "runner", id, "success", map[string]any{"name": req.Name})
	writeJSON(w, 201, map[string]string{"runner_id": id, "status": "pending"})
}

func handleUpdateRunner(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.updated", "runner", id, authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	var req struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status != "active" && req.Status != "paused" {
		writeJSON(w, 400, map[string]string{"error": "invalid status"})
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE runners SET name=?, status=? WHERE id=? AND organisation_id=? AND status != 'revoked'", req.Name, req.Status, id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update runner"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.updated", "runner", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func handleRevokeRunner(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.revoked", "runner", id, authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE runners SET status='revoked', revoked_at="+apiCurrentTime()+" WHERE id=? AND organisation_id=? AND status != 'revoked'", id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to revoke runner"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	if _, err := apiExec(r.Context(), dbFrom(r), "UPDATE runner_credentials SET revoked_at = "+apiCurrentTime()+" WHERE runner_id = ? AND revoked_at IS NULL", id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runner revoked but credential revocation failed"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.revoked", "runner", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}

func handleRegisterRunner(w http.ResponseWriter, r *http.Request) {
	orgID, credentialRunnerID, err := authenticateRunnerCredential(dbFromRequest(r), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
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

	db := dbFromRequest(r)
	if credentialRunnerID != "" {
		now := time.Now().UTC()
		result, updateErr := apiExec(r.Context(), db,
			`UPDATE runners SET status='active', version=?, hostname=?, platform=?, ip_address=?, registered_at=COALESCE(registered_at,?), last_seen_at=?, updated_at=? WHERE id=? AND organisation_id=? AND status != 'revoked'`,
			sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform), sqlNullString(req.IPAddress), now, now, now, credentialRunnerID, orgID)
		if updateErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register runner"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "runner is not available for registration"})
			return
		}
		runnerID = credentialRunnerID
	} else {
		now := time.Now().UTC()
		_, err = apiExec(r.Context(), db,
			`INSERT INTO runners (id, organisation_id, name, runner_type, status, version, hostname, platform, ip_address, registered_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			runnerID, orgID, req.Name, req.RunnerType, "active",
			sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform),
			sqlNullString(req.IPAddress), now)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to register runner: " + err.Error()})
			return
		}
	}

	if _, err := apiExec(r.Context(), db,
		"INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING",
		"rsc_"+shortID(), orgID, runnerID, "all", "*"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign runner scope"})
		return
	}

	// Registration is the handshake that turns an organisation-wide bootstrap
	// credential into an identity-bound one. Everything the runner does after
	// this point (claiming work, renewing leases, submitting results) requires
	// the bound credential, so one runner cannot act as another.
	boundToken, err := createRunnerCredential(r.Context(), db, orgID, "", runnerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to issue runner credential"})
		return
	}

	writeAuditEvent(r.Context(), db, orgID, "", "runner.registered", "runner", runnerID, "success", map[string]any{"name": req.Name})
	writeJSON(w, 201, map[string]any{
		"runner_id":       runnerID,
		"organisation_id": orgID,
		"status":          "active",
		"runner_token":    boundToken,
	})
}

func handleRunnerHeartbeat(w http.ResponseWriter, r *http.Request) {
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
	orgID, err := authenticateBoundRunner(dbFromRequest(r), r, req.RunnerID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	result, err := apiExec(r.Context(), dbFromRequest(r),
		"UPDATE runners SET status = 'active', last_seen_at = ?, hostname = COALESCE(NULLIF(?,''), hostname), platform = COALESCE(NULLIF(?,''), platform), version = COALESCE(NULLIF(?,''), version) WHERE id = ? AND organisation_id = ?",
		now, req.Hostname, req.Platform, req.Version, req.RunnerID, orgID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.credential.rotated", "runner", "", authz.Deny("runner_management_required", "Creating runner credentials requires a privileged role."))
		return
	}
	var req struct {
		RunnerID string `json:"runner_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	token, err := createRunnerCredential(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, req.RunnerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{
		"registration_token": token,
		"expires_in":         "3600",
	})
}

// registrationCredentialTTL bounds an unbound bootstrap credential. It is
// short because it only has to survive long enough for a runner to complete
// the registration handshake.
const registrationCredentialTTL = time.Hour

// boundCredentialTTL bounds a runner-bound credential. It is long because the
// runner uses it for its whole working life, and it re-registers to rotate.
const boundCredentialTTL = 30 * 24 * time.Hour

func createRunnerCredential(ctx context.Context, db *sql.DB, orgID, actorID, runnerID string) (string, error) {
	if runnerID != "" {
		var status string
		if err := apiQueryRow(ctx, db, "SELECT status FROM runners WHERE id = ? AND organisation_id = ?", runnerID, orgID).Scan(&status); err != nil {
			return "", fmt.Errorf("runner not found")
		}
		if status == "revoked" {
			return "", fmt.Errorf("runner is revoked")
		}
		if _, err := apiExec(ctx, db, "UPDATE runner_credentials SET revoked_at = "+apiCurrentTime()+" WHERE runner_id = ? AND revoked_at IS NULL", runnerID); err != nil {
			return "", fmt.Errorf("failed to revoke previous runner credentials")
		}
	}
	token := newToken()
	if token == "" {
		return "", fmt.Errorf("failed to generate registration token")
	}
	ttl := registrationCredentialTTL
	if runnerID != "" {
		ttl = boundCredentialTTL
	}
	expiresAt := time.Now().UTC().Add(ttl)
	if _, err := apiExec(ctx, db, "INSERT INTO runner_credentials (id, organisation_id, runner_id, token_hash, expires_at) VALUES (?,?,?,?,?)", "rct_"+shortID(), orgID, sqlNullString(runnerID), hashToken(token), expiresAt); err != nil {
		return "", fmt.Errorf("failed to create registration token")
	}
	writeAuditEvent(ctx, db, orgID, actorID, "runner.credential.rotated", "runner", runnerID, "success", map[string]any{"expires_in": int(ttl.Seconds())})
	return token, nil
}

func handleRotateRunnerToken(w http.ResponseWriter, r *http.Request, runnerID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.credential.rotated", "runner", runnerID, authz.Deny("runner_management_required", "Rotating runner credentials requires a privileged role."))
		return
	}
	if runnerID == "" || strings.Contains(runnerID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runner id"})
		return
	}
	token, err := createRunnerCredential(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, runnerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"registration_token": token, "expires_in": "3600", "runner_id": runnerID})
}
