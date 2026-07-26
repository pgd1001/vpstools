package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/redact"
	"github.com/pgd1001/svrtools/packages/runbooks"
)

func handleCreateRunbook(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "runbook.created", "runbook", "", authz.Deny("runbook_requires_senior", "Creating runbooks requires senior engineer or above."))
		return
	}

	var req struct {
		Name         string `json:"name"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Risk         string `json:"risk"`
		Command      string `json:"command"`
		Timeout      int    `json:"timeout"`
		Environment  string `json:"environment"`
		YAML         string `json:"yaml"`
		AllowedRoles string `json:"allowed_roles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}

	var rb *runbooks.Runbook
	if req.YAML != "" {
		var err error
		rb, err = runbooks.Parse(req.YAML)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
	} else {
		if req.Name == "" || req.Command == "" {
			writeJSON(w, 400, map[string]string{"error": "name and command are required"})
			return
		}
		if req.Risk == "" {
			req.Risk = "medium"
		}
		if req.Timeout == 0 {
			req.Timeout = 300
		}
		if !validRisk(req.Risk) || !validEnvironment(req.Environment) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid risk or environment"})
			return
		}
		rb = &runbooks.Runbook{
			APIVersion: "vps-tools.io/v1",
			Kind:       "Runbook",
			Metadata: runbooks.Metadata{
				Name:        req.Name,
				Title:       req.Title,
				Description: req.Description,
				Risk:        runbooks.RiskLevel(req.Risk),
				Version:     1,
			},
			Spec: runbooks.Spec{
				Execution: runbooks.Execution{
					Command:        req.Command,
					TimeoutSeconds: req.Timeout,
				},
				Targets: runbooks.TargetRules{
					AllowedEnvironments: []string{req.Environment},
				},
			},
		}
	}
	if req.AllowedRoles != "" && !validAllowedRoles(req.AllowedRoles) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "allowed_roles must be a JSON array of known roles"})
		return
	}

	db := dbFrom(r)
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start runbook transaction"})
		return
	}
	defer tx.Rollback()
	rbID := "rbk_" + shortID()
	yamlBytes, _ := json.Marshal(rb)
	defJSON, _ := json.Marshal(rb)

	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO runbooks (id, organisation_id, name, title, description, status, created_by_user_id)
		VALUES (?,?,?,?,?,'draft',?)`,
		rbID, actor.OrganisationID, rb.Metadata.Name, rb.Metadata.Title, rb.Metadata.Description, actor.UserID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "runbook name already exists or could not be created"})
		return
	}

	rvID := "rbv_" + shortID()
	targetJSON := jsonString(rb.Spec.Targets)
	approvalJSON := jsonString(rb.Spec.Approval)

	allowedRoles := `["senior_engineer","admin","owner"]`
	if req.AllowedRoles != "" {
		allowedRoles = req.AllowedRoles
	}

	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO runbook_versions (id, organisation_id, runbook_id, version, status, risk_level, allowed_roles, definition_yaml, definition_json, command_preview, command_hash, target_constraints, parameter_schema, approval_rules, created_by_user_id)
		VALUES (?,?,?,?,'draft',?,?,?,?,?,?,?,?,?,?)`,
		rvID, actor.OrganisationID, rbID, rb.Metadata.Version, string(rb.Metadata.Risk),
		allowedRoles, string(yamlBytes), string(defJSON), rb.Spec.Execution.Command, hashCmd(rb.Spec.Execution.Command),
		targetJSON, "{}", approvalJSON, actor.UserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create runbook version"})
		return
	}

	if _, err = tx.ExecContext(r.Context(), "UPDATE runbooks SET current_version_id = ? WHERE id = ?", rvID, rbID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to activate runbook version"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit runbook"})
		return
	}

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runbook.created", "runbook", rbID, "success", map[string]any{"name": rb.Metadata.Name})
	writeJSON(w, 201, map[string]any{"runbook_id": rbID, "name": rb.Metadata.Name, "status": "draft"})
}

func handleListRunbooks(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	search := r.URL.Query().Get("search")

	query := `SELECT r.id, r.name, r.title, r.description, r.status,
		COALESCE(rv.risk_level,'medium'), COALESCE(rv.command_preview,''), COALESCE(rv.allowed_roles,'["senior_engineer","admin","owner"]'), r.created_at
		FROM runbooks r
		LEFT JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE r.organisation_id = ? AND r.status != 'archived'`
	args := []any{actor.OrganisationID}
	if search != "" {
		query += ` AND (r.name LIKE ? OR r.title LIKE ? OR COALESCE(r.description,'') LIKE ?)`
		like := "%" + search + "%"
		args = append(args, like, like, like)
	}
	query += ` ORDER BY r.name ASC`

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type rbItem struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Status       string `json:"status"`
		Risk         string `json:"risk_level"`
		Command      string `json:"command_preview"`
		CreatedAt    string `json:"created_at"`
		Permitted    bool   `json:"permitted"`
		AllowedRoles string `json:"allowed_roles"`
	}

	var results []rbItem
	for rows.Next() {
		var item rbItem
		rows.Scan(&item.ID, &item.Name, &item.Title, &item.Description, &item.Status,
			&item.Risk, &item.Command, &item.AllowedRoles, &item.CreatedAt)
		item.Permitted = roleAllowedInList(actor.Role, item.AllowedRoles)
		results = append(results, item)
	}
	writeJSON(w, 200, map[string]any{"runbooks": results})
}

func handleGetRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var rb struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		Status       string `json:"status"`
		Version      int    `json:"version"`
		Risk         string `json:"risk_level"`
		Command      string `json:"command"`
		Definition   string `json:"definition_json"`
		AllowedRoles string `json:"allowed_roles"`
		CreatedAt    string `json:"created_at"`
	}

	err := db.QueryRowContext(r.Context(),
		`SELECT r.id, r.name, r.title, COALESCE(r.description,''), r.status,
		COALESCE(rv.version,1), COALESCE(rv.risk_level,'medium'), COALESCE(rv.command_preview,''),
		COALESCE(rv.definition_json,'{}'), COALESCE(rv.allowed_roles,'["senior_engineer","admin","owner"]'), r.created_at
		FROM runbooks r
		LEFT JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE (r.id = ? OR r.name = ?) AND r.organisation_id = ?`,
		name, name, actor.OrganisationID,
	).Scan(&rb.ID, &rb.Name, &rb.Title, &rb.Description, &rb.Status,
		&rb.Version, &rb.Risk, &rb.Command, &rb.Definition, &rb.AllowedRoles, &rb.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runbook not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"runbook": rb})
}

func handlePublishRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "runbook.published", "runbook", "", authz.Deny("publish_requires_senior", "Publishing runbooks requires senior engineer or above."))
		return
	}

	db := dbFrom(r)
	var rbID, rvID string
	var version int
	err := db.QueryRowContext(r.Context(),
		`SELECT r.id, rv.id, rv.version FROM runbooks r
		JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE (r.id = ? OR r.name = ?) AND r.organisation_id = ?
		LIMIT 1`,
		name, name, actor.OrganisationID).Scan(&rbID, &rvID, &version)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runbook not found"})
		return
	}

	db.ExecContext(r.Context(), "UPDATE runbooks SET status = 'published' WHERE id = ?", rbID)
	db.ExecContext(r.Context(), "UPDATE runbook_versions SET status = 'published', published_by_user_id = ?, published_at = datetime('now') WHERE id = ?", actor.UserID, rvID)

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runbook.published", "runbook", rbID, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "published"})
}

// Updating a runbook creates a new draft version. Published versions remain immutable.
func handleUpdateRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "runbook.updated", "runbook", name, authz.Deny("runbook_requires_senior", "Updating runbooks requires senior engineer or above."))
		return
	}
	var req struct {
		Title, Description, Risk, Command, Environment, YAML, AllowedRoles string
		Timeout                                                            int `json:"timeout"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	db := dbFrom(r)
	var rbID, currentName string
	var currentVersion int
	if err := db.QueryRowContext(r.Context(), `SELECT r.id,r.name,COALESCE(rv.version,0) FROM runbooks r LEFT JOIN runbook_versions rv ON rv.id=r.current_version_id WHERE (r.id=? OR r.name=?) AND r.organisation_id=? AND r.status != 'archived'`, name, name, actor.OrganisationID).Scan(&rbID, &currentName, &currentVersion); err != nil {
		writeJSON(w, 404, map[string]string{"error": "runbook not found"})
		return
	}
	if req.YAML == "" && strings.TrimSpace(req.Command) == "" {
		writeJSON(w, 400, map[string]string{"error": "command or yaml is required"})
		return
	}
	var rb *runbooks.Runbook
	if req.YAML != "" {
		var err error
		rb, err = runbooks.Parse(req.YAML)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
	} else {
		if req.Risk == "" {
			req.Risk = "medium"
		}
		if req.Timeout == 0 {
			req.Timeout = 300
		}
		if req.Environment == "" {
			req.Environment = "development"
		}
		if !validRisk(req.Risk) || !validEnvironment(req.Environment) {
			writeJSON(w, 400, map[string]string{"error": "invalid risk or environment"})
			return
		}
		rb = &runbooks.Runbook{APIVersion: "vps-tools.io/v1", Kind: "Runbook", Metadata: runbooks.Metadata{Name: currentName, Title: req.Title, Description: req.Description, Risk: runbooks.RiskLevel(req.Risk), Version: currentVersion + 1}, Spec: runbooks.Spec{Execution: runbooks.Execution{Command: req.Command, TimeoutSeconds: req.Timeout}, Targets: runbooks.TargetRules{AllowedEnvironments: []string{req.Environment}}}}
	}
	if rb.Metadata.Name == "" {
		rb.Metadata.Name = currentName
	}
	rb.Metadata.Version = currentVersion + 1
	if req.AllowedRoles != "" && !validAllowedRoles(req.AllowedRoles) {
		writeJSON(w, 400, map[string]string{"error": "allowed_roles must be a JSON array of known roles"})
		return
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to start update"})
		return
	}
	defer tx.Rollback()
	rvID := "rbv_" + shortID()
	def, _ := json.Marshal(rb)
	allowed := `["senior_engineer","admin","owner"]`
	if req.AllowedRoles != "" {
		allowed = req.AllowedRoles
	}
	if req.Title == "" {
		req.Title = rb.Metadata.Title
	}
	if req.Description == "" {
		req.Description = rb.Metadata.Description
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO runbook_versions (id,organisation_id,runbook_id,version,status,risk_level,allowed_roles,definition_yaml,definition_json,command_preview,command_hash,target_constraints,parameter_schema,approval_rules,created_by_user_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, rvID, actor.OrganisationID, rbID, rb.Metadata.Version, "draft", string(rb.Metadata.Risk), allowed, string(def), string(def), rb.Spec.Execution.Command, hashCmd(rb.Spec.Execution.Command), jsonString(rb.Spec.Targets), "{}", jsonString(rb.Spec.Approval), actor.UserID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create runbook version"})
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE runbooks SET title=?,description=?,status='draft',current_version_id=? WHERE id=? AND organisation_id=?", req.Title, req.Description, rvID, rbID, actor.OrganisationID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update runbook"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to commit update"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runbook.updated", "runbook", rbID, "success", map[string]any{"version": rb.Metadata.Version})
	writeJSON(w, 200, map[string]any{"status": "draft", "version": rb.Metadata.Version})
}

func handleArchiveRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "runbook.archived", "runbook", name, authz.Deny("runbook_requires_senior", "Archiving runbooks requires senior engineer or above."))
		return
	}
	res, err := dbFrom(r).ExecContext(r.Context(), "UPDATE runbooks SET status='archived' WHERE (id=? OR name=?) AND organisation_id=? AND status != 'archived'", name, name, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to archive runbook"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "runbook not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runbook.archived", "runbook", name, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

func handleRunRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var req struct {
		Target string            `json:"target"`
		Reason string            `json:"reason"`
		Params map[string]string `json:"params"`
		DryRun bool              `json:"dry_run"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var rbID, rvID, command, definitionJSON string
	var risk, allowedRoles string
	var targetJSON string
	err := db.QueryRowContext(r.Context(),
		`SELECT r.id, rv.id, rv.command_preview, rv.risk_level, rv.target_constraints, COALESCE(rv.allowed_roles,'["senior_engineer","admin","owner"]'), rv.definition_json
		FROM runbooks r
		JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE (r.id = ? OR r.name = ?) AND r.organisation_id = ? AND r.status = 'published'`,
		name, name, actor.OrganisationID).Scan(&rbID, &rvID, &command, &risk, &targetJSON, &allowedRoles, &definitionJSON)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runbook not found or not published"})
		return
	}

	if rbID == "" {
		writeJSON(w, 404, map[string]string{"error": "runbook not found or not published"})
		return
	}

	if !roleAllowedInList(actor.Role, allowedRoles) {
		writeDenial(w, r, actor, "runbook.executed", "runbook", rbID, authz.Deny("runbook_not_permitted", "This runbook is not permitted for your role. Use 'vps runbook list' to see permitted runbooks."))
		return
	}

	var rbDef runbooks.Runbook
	if err := json.Unmarshal([]byte(definitionJSON), &rbDef); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored runbook definition"})
		return
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
	if !rbDef.EnvironmentAllowed(env) || !runbookTargetsAllowed(r.Context(), db, actor.OrganisationID, rbDef, targetIDs) {
		writeDenial(w, r, actor, "runbook.executed", "runbook", rbID, authz.Deny("runbook_target_not_permitted", "This runbook is not permitted for the selected target."))
		return
	}
	var renderErr error
	command, renderErr = rbDef.RenderCommand(req.Params)
	if renderErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": renderErr.Error()})
		return
	}
	if command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runbook rendered an empty command"})
		return
	}
	targetSnapshot := snapshotTargets(r.Context(), db, actor.OrganisationID, targetIDs)
	if len(targetSnapshot) != len(targetIDs) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to snapshot selected targets"})
		return
	}

	// Check if approval is needed (high-risk production, or junior on high-risk)
	needsApproval := false
	if rbDef.NeedsApproval(env) || env == "production" && (risk == "high" || risk == "critical") {
		needsApproval = true
	}
	if risk == "high" && !actor.IsSenior() {
		needsApproval = true
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "preflight", "approval_required": needsApproval,
			"environment": env, "risk_level": risk, "target_count": len(targetIDs),
			"target_snapshot": targetSnapshot, "command_preview": redact.Stdout(command),
		})
		return
	}
	if needsApproval {
		if strings.TrimSpace(req.Reason) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "an approval reason is required"})
			return
		}
		approvalID := "apr_" + shortID()
		execPayload := jsonString(map[string]any{
			"runbook_id":      rbID,
			"runbook_name":    name,
			"target":          req.Target,
			"command":         command,
			"risk":            risk,
			"reason":          req.Reason,
			"params":          req.Params,
			"target_ids":      targetIDs,
			"target_snapshot": targetSnapshot,
			"environment":     env,
			"target_count":    len(targetIDs),
			"timeout": func() int {
				if rbDef.Spec.Execution.TimeoutSeconds > 0 {
					return rbDef.Spec.Execution.TimeoutSeconds
				}
				return 300
			}(),
		})
		tx, txErr := db.BeginTx(r.Context(), nil)
		if txErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start approval transaction"})
			return
		}
		defer tx.Rollback()
		if _, txErr = tx.ExecContext(r.Context(),
			`INSERT INTO approval_requests (id, organisation_id, requester_user_id, action_type, status, risk_level, reason, target_type, target_id, target_snapshot, request_payload, expires_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,datetime('now','+1 hour'))`,
			approvalID, actor.OrganisationID, actor.UserID, "runbook", "pending", risk, req.Reason, "server", req.Target, jsonString(targetSnapshot), execPayload); txErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create approval request"})
			return
		}
		if err := writeAuditEventTx(r.Context(), tx, actor.OrganisationID, actor.UserID, "approval.requested", "approval", approvalID, "pending", map[string]any{"runbook_id": rbID, "target_count": len(targetIDs)}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record approval audit event"})
			return
		}
		if txErr = tx.Commit(); txErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit approval request"})
			return
		}
		writeJSON(w, 202, map[string]string{
			"status":      "awaiting_approval",
			"approval_id": approvalID,
			"message":     "This runbook requires approval. Use 'vps approvals approve " + approvalID + "' to proceed.",
		})
		return
	}

	// Policy check for direct runbook execution (no approval needed)
	dec := policy.CheckRunbookExecution(actor, authz.Env(env), authz.RiskLevel(risk), req.Reason, map[string]any{})
	if !dec.Allowed {
		writeDenial(w, r, actor, "runbook.executed", "runbook", rbID, dec)
		return
	}

	// Execute directly
	execID := "exe_" + shortID()
	timeout := rbDef.Spec.Execution.TimeoutSeconds
	if timeout <= 0 {
		timeout = 300
	}
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start execution transaction"})
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, execution_type, status, environment, risk_level, reason, command, command_preview, command_hash, timeout_seconds)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		execID, actor.OrganisationID, actor.UserID, actor.Role, "runbook", "queued", env, risk, req.Reason, command, redact.Stdout(command), hashCmd(command), timeout); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create execution"})
		return
	}

	for i, srvID := range targetIDs {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?,?,?,?,'pending',?)`,
			"ext_"+shortID(), actor.OrganisationID, execID, srvID, jsonString(targetSnapshot[i])); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create execution target"})
			return
		}
	}
	if err = recordExecutionEvent(r.Context(), tx, actor.OrganisationID, execID, "", "", "queued", "runbook.queued", map[string]any{"runbook_id": rbID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}

	if err = writeAuditEventTx(r.Context(), tx, actor.OrganisationID, actor.UserID, "runbook.executed", "runbook", rbID, "queued", map[string]any{"execution_id": execID, "command": redact.Stdout(command)}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution audit event"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit execution"})
		return
	}
	writeJSON(w, 201, map[string]any{
		"execution_id": execID,
		"status":       "queued",
		"target_count": len(targetIDs),
	})
}

func handleListApprovals(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	status := r.URL.Query().Get("status")

	query := `SELECT a.id, COALESCE(u.display_name,a.requester_user_id), a.action_type, a.status, a.risk_level, a.reason, a.target_type, COALESCE(a.target_id,''), a.expires_at, a.created_at, a.target_snapshot, COALESCE(a.decision_note,'')
		FROM approval_requests a LEFT JOIN users u ON u.id = a.requester_user_id WHERE a.organisation_id = ?`
	args := []any{actor.OrganisationID}
	if status != "" {
		query += " AND a.status = ?"
		args = append(args, status)
	} else {
		query += " AND a.status = 'pending'"
	}
	query += " ORDER BY a.created_at DESC LIMIT 50"

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type approval struct {
		ID             string `json:"id"`
		RequesterName  string `json:"requester_name"`
		ActionType     string `json:"action_type"`
		Status         string `json:"status"`
		RiskLevel      string `json:"risk_level"`
		Reason         string `json:"reason"`
		TargetType     string `json:"target_type"`
		TargetID       string `json:"target_id"`
		ExpiresAt      string `json:"expires_at"`
		CreatedAt      string `json:"created_at"`
		TargetSnapshot string `json:"target_snapshot"`
		DecisionNote   string `json:"decision_note"`
	}

	results := []approval{}
	for rows.Next() {
		var a approval
		if err := rows.Scan(&a.ID, &a.RequesterName, &a.ActionType, &a.Status, &a.RiskLevel,
			&a.Reason, &a.TargetType, &a.TargetID, &a.ExpiresAt, &a.CreatedAt, &a.TargetSnapshot, &a.DecisionNote); err != nil {
			continue
		}
		results = append(results, a)
	}
	writeJSON(w, 200, map[string]any{"approvals": results})
}

func handleGetApproval(w http.ResponseWriter, r *http.Request, approvalID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	var approval map[string]any
	var id, requesterID, requesterName, actionType, status, risk, reason, targetType, targetID, targetSnapshot, payload, expiresAt, createdAt, decidedAt, decisionNote string
	err := db.QueryRowContext(r.Context(), `SELECT a.id, a.requester_user_id, COALESCE(u.display_name,a.requester_user_id), a.action_type, a.status, a.risk_level, a.reason, a.target_type, COALESCE(a.target_id,''), a.target_snapshot, a.request_payload, a.expires_at, a.created_at, COALESCE(a.decided_at,''), COALESCE(a.decision_note,'')
		FROM approval_requests a LEFT JOIN users u ON u.id = a.requester_user_id
		WHERE a.id = ? AND a.organisation_id = ?`, approvalID, actor.OrganisationID).Scan(&id, &requesterID, &requesterName, &actionType, &status, &risk, &reason, &targetType, &targetID, &targetSnapshot, &payload, &expiresAt, &createdAt, &decidedAt, &decisionNote)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
		return
	}
	approval = map[string]any{"id": id, "requester_id": requesterID, "requester_name": requesterName, "action_type": actionType, "status": status, "risk_level": risk, "reason": reason, "target_type": targetType, "target_id": targetID, "target_snapshot": targetSnapshot, "request_payload": redactedApprovalPayload(payload), "expires_at": expiresAt, "created_at": createdAt, "decided_at": decidedAt, "decision_note": decisionNote}
	writeJSON(w, http.StatusOK, map[string]any{"approval": approval})
}

func redactedApprovalPayload(raw string) map[string]any {
	view := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		return map[string]any{"available": false}
	}
	if command, ok := view["command"].(string); ok {
		view["command_preview"] = redact.Stdout(command)
		delete(view, "command")
	}
	if params, ok := view["params"].(map[string]any); ok {
		for key, value := range params {
			params[key] = redact.Stdout(fmt.Sprint(value))
		}
	}
	return view
}

func handleApprove(w http.ResponseWriter, r *http.Request, approvalID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "approval.approved", "approval", approvalID, authz.Deny("approve_requires_senior", "Approving requires senior engineer or above."))
		return
	}

	var note string
	var body struct {
		Note string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&body) == nil {
		note = body.Note
	}

	db := dbFrom(r)

	var payload string
	var requesterID, riskLevel, reason, expiresAt string
	err := db.QueryRowContext(r.Context(),
		"SELECT requester_user_id, risk_level, reason, request_payload, expires_at FROM approval_requests WHERE id = ? AND organisation_id = ?",
		approvalID, actor.OrganisationID).Scan(&requesterID, &riskLevel, &reason, &payload, &expiresAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "approval not found"})
		return
	}
	if requesterID == actor.UserID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "requesters cannot approve their own requests"})
		return
	}
	var expired int
	db.QueryRowContext(r.Context(), "SELECT CASE WHEN expires_at <= datetime('now') THEN 1 ELSE 0 END FROM approval_requests WHERE id = ?", approvalID).Scan(&expired)
	if expired == 1 || expiresAt == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "approval has expired"})
		return
	}

	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start approval transaction"})
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(),
		"UPDATE approval_requests SET status = 'approved', approver_user_id = ?, decided_at = datetime('now'), decision_note = ? WHERE id = ? AND organisation_id = ? AND status = 'pending'",
		actor.UserID, sqlNullString(note), approvalID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to approve"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "approval not found or already decided"})
		return
	}

	execID, execErr := createExecutionFromApprovalTx(r.Context(), tx, actor.OrganisationID, requesterID, actor.UserID, riskLevel, reason, payload, approvalID)
	if execErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create execution from approval"})
		return
	}
	if err = writeAuditEventTx(r.Context(), tx, actor.OrganisationID, actor.UserID, "approval.approved", "approval", approvalID, "approved", map[string]any{"execution_id": execID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record approval audit event"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit approval"})
		return
	}
	resp := map[string]string{"status": "approved"}
	if execID != "" {
		resp["execution_id"] = execID
		resp["message"] = "Execution " + execID + " has been queued."
	}
	writeJSON(w, 200, resp)
}

func createExecutionFromApprovalTx(ctx context.Context, tx *sql.Tx, orgID, requesterID, approverID, risk, reason, payload, approvalID string) (string, error) {
	var intent struct {
		Command        string           `json:"command"`
		RunbookID      string           `json:"runbook_id"`
		RunbookName    string           `json:"runbook_name"`
		TargetIDs      []string         `json:"target_ids"`
		TargetSnapshot []map[string]any `json:"target_snapshot"`
		Environment    string           `json:"environment"`
		Target         string           `json:"target"`
		Timeout        int              `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(payload), &intent); err != nil || intent.Command == "" {
		return "", fmt.Errorf("invalid approval payload")
	}

	env := intent.Environment
	if env == "" {
		env = "development"
	}
	if intent.Timeout <= 0 {
		intent.Timeout = 300
	}

	execID := "exe_" + shortID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, delegated_by_user_id, approval_id, execution_type, status, environment, risk_level, reason, command, command_preview, command_hash, timeout_seconds)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		execID, orgID, requesterID, "junior_engineer", approverID, approvalID, "runbook", "queued", env, risk, reason, intent.Command, redact.Stdout(intent.Command), hashCmd(intent.Command), intent.Timeout); err != nil {
		return "", err
	}

	for i, srvID := range intent.TargetIDs {
		serverSnapshot := "{}"
		if i < len(intent.TargetSnapshot) {
			serverSnapshot = jsonString(intent.TargetSnapshot[i])
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?,?,?,?,'pending',?)`,
			"ext_"+shortID(), orgID, execID, srvID, serverSnapshot); err != nil {
			return "", err
		}
	}
	if err := recordExecutionEvent(ctx, tx, orgID, execID, "", "", "queued", "runbook.queued", map[string]any{"runbook_id": intent.RunbookID}); err != nil {
		return "", err
	}

	if err := writeAuditEventTx(ctx, tx, orgID, requesterID, "runbook.executed", "runbook", intent.RunbookID, "queued",
		map[string]any{"execution_id": execID, "command": redact.Stdout(intent.Command), "delegated_by": approverID}); err != nil {
		return "", err
	}
	return execID, nil
}

func handleDeny(w http.ResponseWriter, r *http.Request, approvalID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "approval.denied", "approval", approvalID, authz.Deny("deny_requires_senior", "Denying requires senior engineer or above."))
		return
	}

	var note string
	var req struct {
		Note string `json:"note"`
	}
	if json.NewDecoder(r.Body).Decode(&req) == nil {
		note = req.Note
	}

	db := dbFrom(r)
	result, err := db.ExecContext(r.Context(),
		"UPDATE approval_requests SET status = 'denied', approver_user_id = ?, decided_at = datetime('now'), decision_note = ? WHERE id = ? AND organisation_id = ? AND status = 'pending'",
		actor.UserID, sqlNullString(note), approvalID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to deny"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "approval not found"})
		return
	}

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "approval.denied", "approval", approvalID, "denied", nil)
	writeJSON(w, 200, map[string]string{"status": "denied"})
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func roleAllowedInList(userRole string, allowedRolesJSON string) bool {
	if allowedRolesJSON == "" {
		return false
	}
	var roles []string
	if err := json.Unmarshal([]byte(allowedRolesJSON), &roles); err != nil {
		return false
	}
	for _, r := range roles {
		if strings.EqualFold(r, userRole) {
			return true
		}
	}
	return false
}

func validRisk(risk string) bool {
	switch strings.ToLower(risk) {
	case "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func validEnvironment(env string) bool {
	return env == "" || env == "*" || env == "development" || env == "staging" || env == "production"
}

func validAllowedRoles(raw string) bool {
	var roles []string
	if err := json.Unmarshal([]byte(raw), &roles); err != nil || len(roles) == 0 {
		return false
	}
	known := map[string]bool{"owner": true, "admin": true, "senior_engineer": true, "junior_engineer": true, "auditor": true}
	for _, role := range roles {
		if !known[strings.ToLower(role)] {
			return false
		}
	}
	return true
}

func runbookTargetsAllowed(ctx context.Context, db *sql.DB, orgID string, rb runbooks.Runbook, targetIDs []string) bool {
	for _, serverID := range targetIDs {
		var name string
		if err := db.QueryRowContext(ctx, "SELECT name FROM servers WHERE id = ? AND organisation_id = ?", serverID, orgID).Scan(&name); err != nil {
			return false
		}
		if len(rb.Spec.Targets.AllowedServers) > 0 {
			allowed := false
			for _, candidate := range rb.Spec.Targets.AllowedServers {
				if candidate == serverID || candidate == name || candidate == "*" {
					allowed = true
				}
			}
			if !allowed {
				return false
			}
		}
		for key, value := range rb.Spec.Targets.AllowedTags {
			var found int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM server_tags WHERE organisation_id = ? AND server_id = ? AND key = ? AND value = ?", orgID, serverID, key, value).Scan(&found); err != nil || found == 0 {
				return false
			}
		}
	}
	return true
}
