package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Audit completeness is a stated invariant: every privileged action must be
// discoverable. Events written by the system rather than a person have a NULL
// actor, and those were silently dropped by the search handler because the
// NULL failed to scan into a string. That hid exactly the automated actions
// the audit trail exists to record.
func TestAuditSearchReturnsSystemActorEvents(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	// A system event, as written by the runner result path.
	if _, err := db.Exec(`INSERT INTO audit_events (id, organisation_id, actor_user_id, actor_type, action, target_type, target_id, result, metadata, occurred_at, previous_hash, event_hash)
		VALUES ('aud_system','org_demo',NULL,'system','execution.completed','execution','exe_sys','succeeded','{}',datetime('now'),'','h1')`); err != nil {
		t.Fatalf("insert system audit event: %v", err)
	}
	// A user event, for contrast.
	if _, err := db.Exec(`INSERT INTO audit_events (id, organisation_id, actor_user_id, actor_type, action, target_type, target_id, result, metadata, occurred_at, previous_hash, event_hash)
		VALUES ('aud_user','org_demo','user_senior','user','execution.requested','execution','exe_usr','queued','{}',datetime('now'),'h1','h2')`); err != nil {
		t.Fatalf("insert user audit event: %v", err)
	}

	w := doRequest(t, mux, http.MethodGet, "/api/v1/audit?limit=50", "", "user_senior")
	if w.Code != http.StatusOK {
		t.Fatalf("audit search = %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Events []struct {
			ID      string `json:"id"`
			ActorID string `json:"actor_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}

	found := map[string]bool{}
	for _, event := range response.Events {
		found[event.ID] = true
	}
	if !found["aud_system"] {
		t.Error("audit search dropped the system-actor event")
	}
	if !found["aud_user"] {
		t.Error("audit search dropped the user event")
	}
}

// A NULL target_type or target_id must not drop the event either.
func TestAuditSearchReturnsEventsWithoutATarget(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	if _, err := db.Exec(`INSERT INTO audit_events (id, organisation_id, actor_user_id, actor_type, action, target_type, target_id, result, metadata, occurred_at, previous_hash, event_hash)
		VALUES ('aud_notarget','org_demo','user_senior','user','auth.login',NULL,NULL,'success','{}',datetime('now'),'','h1')`); err != nil {
		t.Fatalf("insert targetless audit event: %v", err)
	}

	w := doRequest(t, mux, http.MethodGet, "/api/v1/audit?limit=50", "", "user_senior")
	if w.Code != http.StatusOK {
		t.Fatalf("audit search = %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Events []struct {
			ID string `json:"id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	for _, event := range response.Events {
		if event.ID == "aud_notarget" {
			return
		}
	}
	t.Error("audit search dropped an event with no target")
}

// The audit trail written by a real execution must be searchable end to end,
// including the runner-submitted completion event.
func TestExecutionLifecycleAuditIsSearchable(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()
	queueJobForClaim(t, mux)

	claim := claimWithToken(t, mux, "rnr_local", "test-runner-token")
	if claim.Code != http.StatusOK {
		t.Fatalf("claim failed: %d %s", claim.Code, claim.Body.String())
	}
	var dispatched struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		LeaseID     string `json:"lease_id"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &dispatched); err != nil {
		t.Fatalf("decode job: %v", err)
	}

	body := `{"execution_id":"` + dispatched.ExecutionID + `","target_id":"` + dispatched.TargetID +
		`","runner_id":"rnr_local","lease_id":"` + dispatched.LeaseID + `","exit_code":0,"stdout":"ok"}`
	result := doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result", body, "user_senior")
	if result.Code != http.StatusOK {
		t.Fatalf("submit result = %d %s", result.Code, result.Body.String())
	}

	w := doRequest(t, mux, http.MethodGet, "/api/v1/audit?limit=50", "", "user_senior")
	var response struct {
		Events []struct {
			Action string `json:"action"`
		} `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	actions := map[string]bool{}
	for _, event := range response.Events {
		actions[event.Action] = true
	}
	// The request is attributed to a user; the completion is written by the
	// system with no actor. Both must be discoverable.
	if !actions["execution.requested"] {
		t.Error("execution.requested is missing from the audit trail")
	}
	if !actions["execution.completed"] {
		t.Error("execution.completed is missing from the audit trail")
	}
}
