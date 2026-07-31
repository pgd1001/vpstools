package main

import (
	"net/http"
	"strings"

	"github.com/pgd1001/svrtools/packages/authz"
)

// Audit search, chain verification, and denial recording.

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

	rows, err := apiQuery(r.Context(), readDBFrom(r), query, args...)
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

func handleVerifyAudit(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() && strings.ToLower(actor.Role) != "auditor" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "audit verification requires an auditor or senior operator"})
		return
	}
	checked, err := verifyAuditHashChain(r.Context(), dbFrom(r), actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"valid": false, "checked_events": checked, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "checked_events": checked})
}

func writeDenial(w http.ResponseWriter, r *http.Request, actor *authz.Actor, action, targetType, targetID string, dec authz.Decision) {
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, action, targetType, targetID, "denied", map[string]any{
		"reason":  dec.Reason,
		"message": dec.Message,
	})
	writeJSON(w, 403, map[string]string{
		"error":  dec.Message,
		"reason": dec.Reason,
		"next":   "Contact your admin or request approval if available.",
	})
}
