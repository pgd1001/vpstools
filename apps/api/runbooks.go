package main

import (
	"encoding/json"
	"net/http"

	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/runbooks"
)

func handleCreateRunbook(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "runbook.created", "runbook", "", authz.Deny("runbook_requires_senior", "Creating runbooks requires senior engineer or above."))
		return
	}

	var req struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Risk        string `json:"risk"`
		Command     string `json:"command"`
		Timeout     int    `json:"timeout"`
		Environment string `json:"environment"`
		YAML        string `json:"yaml"`
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

	db := dbFrom(r)
	rbID := "rbk_" + shortID()
	yamlBytes, _ := json.Marshal(rb)
	defJSON, _ := json.Marshal(rb)

	db.ExecContext(r.Context(),
		`INSERT INTO runbooks (id, organisation_id, name, title, description, status, created_by_user_id)
		VALUES (?,?,?,?,?,'draft',?)`,
		rbID, actor.OrganisationID, rb.Metadata.Name, rb.Metadata.Title, rb.Metadata.Description, actor.UserID)

	rvID := "rbv_" + shortID()
	db.ExecContext(r.Context(),
		`INSERT INTO runbook_versions (id, organisation_id, runbook_id, version, status, risk_level, definition_yaml, definition_json, command_preview, command_hash, target_constraints, parameter_schema, approval_rules, created_by_user_id)
		VALUES (?,?,?,?,'draft',?,?,?,?,?,?,?,?,?)`,
		rvID, actor.OrganisationID, rbID, rb.Metadata.Version, string(rb.Metadata.Risk),
		string(yamlBytes), string(defJSON), rb.Spec.Execution.Command, hashCmd(rb.Spec.Execution.Command),
		jsonString(rb.Spec.Targets), "{}", jsonString(rb.Spec.Approval), actor.UserID)

	db.ExecContext(r.Context(), "UPDATE runbooks SET current_version_id = ? WHERE id = ?", rvID, rbID)

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runbook.created", "runbook", rbID, "success", map[string]any{"name": rb.Metadata.Name})
	writeJSON(w, 201, map[string]any{"runbook_id": rbID, "name": rb.Metadata.Name, "status": "draft"})
}

func handleListRunbooks(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	rows, err := db.QueryContext(r.Context(),
		`SELECT r.id, r.name, r.title, r.description, r.status,
		COALESCE(rv.risk_level,'medium'), COALESCE(rv.command_preview,''), r.created_at
		FROM runbooks r
		LEFT JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE r.organisation_id = ? AND r.status != 'archived'
		ORDER BY r.name ASC`, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type rbItem struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Risk        string `json:"risk_level"`
		Command     string `json:"command_preview"`
		CreatedAt   string `json:"created_at"`
		Permitted   bool   `json:"permitted"`
	}

	var results []rbItem
	for rows.Next() {
		var item rbItem
		rows.Scan(&item.ID, &item.Name, &item.Title, &item.Description, &item.Status,
			&item.Risk, &item.Command, &item.CreatedAt)
		item.Permitted = true
		results = append(results, item)
	}
	writeJSON(w, 200, map[string]any{"runbooks": results})
}

func handleGetRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var rb struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Version     int    `json:"version"`
		Risk        string `json:"risk_level"`
		Command     string `json:"command"`
		Definition  string `json:"definition_json"`
		CreatedAt   string `json:"created_at"`
	}

	err := db.QueryRowContext(r.Context(),
		`SELECT r.id, r.name, r.title, COALESCE(r.description,''), r.status,
		COALESCE(rv.version,1), COALESCE(rv.risk_level,'medium'), COALESCE(rv.command_preview,''), COALESCE(rv.definition_json,'{}'), r.created_at
		FROM runbooks r
		LEFT JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE (r.id = ? OR r.name = ?) AND r.organisation_id = ?`,
		name, name, actor.OrganisationID,
	).Scan(&rb.ID, &rb.Name, &rb.Title, &rb.Description, &rb.Status,
		&rb.Version, &rb.Risk, &rb.Command, &rb.Definition, &rb.CreatedAt)
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

func handleRunRunbook(w http.ResponseWriter, r *http.Request, name string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var req struct {
		Target  string            `json:"target"`
		Reason  string            `json:"reason"`
		Params  map[string]string `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var rbID, rvID, command string
	var risk string
	var targetJSON string
	err := db.QueryRowContext(r.Context(),
		`SELECT r.id, rv.id, rv.command_preview, rv.risk_level, rv.target_constraints
		FROM runbooks r
		JOIN runbook_versions rv ON rv.id = r.current_version_id
		WHERE (r.id = ? OR r.name = ?) AND r.organisation_id = ? AND r.status = 'published'`,
		name, name, actor.OrganisationID).Scan(&rbID, &rvID, &command, &risk, &targetJSON)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runbook not found or not published"})
		return
	}

	if rbID == "" {
		writeJSON(w, 404, map[string]string{"error": "runbook not found or not published"})
		return
	}

	// Parse runbook definition for validation
	var rbDef map[string]any
	json.Unmarshal([]byte(targetJSON), &rbDef)

	targetIDs := resolveTargets(r.Context(), db, actor.OrganisationID, req.Target)
	if len(targetIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no servers found for target: " + req.Target})
		return
	}

	env := detectEnv(r.Context(), db, actor.OrganisationID, targetIDs)

	// Policy check for runbook execution
	dec := policy.CheckRunbookExecution(actor, authz.Env(env), authz.RiskLevel(risk), req.Reason, rbDef)
	if !dec.Allowed {
		writeDenial(w, r, actor, "runbook.executed", "runbook", rbID, dec)
		return
	}

	// Check if approval is needed
	needsApproval := false
	if env == "production" && risk == "high" {
		needsApproval = true
	}
	if needsApproval {
		approvalID := "apr_" + shortID()
		db.ExecContext(r.Context(),
			`INSERT INTO approval_requests (id, organisation_id, requester_user_id, action_type, status, risk_level, reason, target_type, target_id, target_snapshot, request_payload, expires_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,datetime('now','+1 hour'))`,
			approvalID, actor.OrganisationID, actor.UserID, "runbook", "pending", risk, req.Reason, "server", req.Target, "{}", "{}")
		writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "approval.requested", "approval", approvalID, "pending", map[string]any{"runbook_id": rbID})
		writeJSON(w, 202, map[string]string{
			"status":       "awaiting_approval",
			"approval_id":  approvalID,
			"message":      "This runbook requires approval. Use 'vps approvals approve " + approvalID + "' to proceed.",
		})
		return
	}

	// Execute directly
	execID := "exe_" + shortID()
	db.ExecContext(r.Context(),
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, execution_type, status, environment, risk_level, reason, command_preview, command_hash, timeout_seconds)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		execID, actor.OrganisationID, actor.UserID, actor.Role, "runbook", "queued", env, risk, req.Reason, command, hashCmd(command), 300)

	for _, srvID := range targetIDs {
		db.ExecContext(r.Context(),
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status)
			VALUES (?,?,?,?,'pending')`,
			"ext_"+shortID(), actor.OrganisationID, execID, srvID)
	}

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "runbook.executed", "runbook", rbID, "queued", map[string]any{"execution_id": execID, "command": command})
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

	query := `SELECT id, requester_user_id, action_type, status, risk_level, reason, target_type, COALESCE(target_id,''), expires_at, created_at
		FROM approval_requests WHERE organisation_id = ?`
	args := []any{actor.OrganisationID}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	} else {
		query += " AND status = 'pending'"
	}
	query += " ORDER BY created_at DESC LIMIT 50"

	rows, err := db.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type approval struct {
		ID            string `json:"id"`
		RequesterName string `json:"requester_name"`
		ActionType    string `json:"action_type"`
		Status        string `json:"status"`
		RiskLevel     string `json:"risk_level"`
		Reason        string `json:"reason"`
		TargetType    string `json:"target_type"`
		TargetID      string `json:"target_id"`
		ExpiresAt     string `json:"expires_at"`
		CreatedAt     string `json:"created_at"`
	}

	var results []approval
	for rows.Next() {
		var a approval
		rows.Scan(&a.ID, &a.RequesterName, &a.ActionType, &a.Status, &a.RiskLevel,
			&a.Reason, &a.TargetType, &a.TargetID, &a.ExpiresAt, &a.CreatedAt)
		results = append(results, a)
	}
	writeJSON(w, 200, map[string]any{"approvals": results})
}

func handleApprove(w http.ResponseWriter, r *http.Request, approvalID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() {
		writeDenial(w, r, actor, "approval.approved", "approval", approvalID, authz.Deny("approve_requires_senior", "Approving requires senior engineer or above."))
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
		"UPDATE approval_requests SET status = 'approved', approver_user_id = ?, decided_at = datetime('now'), decision_note = ? WHERE id = ? AND organisation_id = ?",
		actor.UserID, sqlNullString(note), approvalID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to approve"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "approval not found"})
		return
	}

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "approval.approved", "approval", approvalID, "approved", nil)
	writeJSON(w, 200, map[string]string{"status": "approved"})
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
		"UPDATE approval_requests SET status = 'denied', approver_user_id = ?, decided_at = datetime('now'), decision_note = ? WHERE id = ? AND organisation_id = ?",
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
