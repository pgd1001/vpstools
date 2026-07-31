package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pgd1001/svrtools/packages/jobsign"
	"github.com/pgd1001/svrtools/packages/redact"
	_ "modernc.org/sqlite"
)

// testJobSigningKey is the signing key used by the API test harness. Job
// dispatch is signed in every configuration, so tests need a real signer.
const testJobSigningKey = "api-test-job-signing-key-at-least-32ch"

func mustTestSigner(t *testing.T) *jobsign.Signer {
	t.Helper()
	signer, err := jobsign.NewSigner(testJobSigningKey)
	if err != nil {
		t.Fatalf("test job signer: %v", err)
	}
	return signer
}

func testAPI(t *testing.T) (*sql.DB, *http.ServeMux, func()) {
	t.Helper()
	t.Setenv("VPS_DEV_AUTH", "true")
	// Dev auth only applies in an explicitly non-production environment.
	t.Setenv("VPS_ENV", "test")
	t.Setenv("APP_ENV", "")
	t.Setenv("ENVIRONMENT", "")
	apiJobSigner = mustTestSigner(t)
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	// Unauthenticated routes (runner registration, heartbeat) resolve their
	// database through the package global rather than request context.
	apiDB = db
	t.Cleanup(func() { apiDB = nil })
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	if err := seed(ctx, db); err != nil {
		db.Close()
		t.Fatalf("seed: %v", err)
	}
	// Bind the test runner credential to rnr_local. Job endpoints require an
	// identity-bound credential, matching what registration issues in
	// production.
	if _, err := db.Exec(`INSERT INTO runner_credentials (id, organisation_id, runner_id, token_hash, expires_at) VALUES ('rct_test','org_demo','rnr_local',?,datetime('now','+1 hour'))`, hashToken("test-runner-token")); err != nil {
		db.Close()
		t.Fatalf("runner credential: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/whoami", withAuth(db, handleWhoAmI))
	mux.HandleFunc("/api/v1/runners/registration-token", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleCreateRegistrationToken(w, r)
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}))
	mux.HandleFunc("/api/v1/runners", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleRegisterRunner(w, r)
			return
		}
		withAuth(db, handleListRunners)(w, r)
	})
	mux.HandleFunc("/api/v1/runners/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/runners/")
		if strings.HasSuffix(id, "/rotate-token") && r.Method == http.MethodPost {
			handleRotateRunnerToken(w, r, strings.TrimSuffix(id, "/rotate-token"))
			return
		}
		if r.Method == http.MethodDelete {
			handleRevokeRunner(w, r, id)
			return
		}
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}))
	mux.HandleFunc("/api/v1/servers", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleListServers(w, r)
			return
		}
		handleAddServer(w, r)
	}))
	mux.HandleFunc("/api/v1/executions", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleListExecutions(w, r)
			return
		}
		handleCreateExecution(w, r)
	}))
	mux.HandleFunc("/api/v1/executions/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/executions/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing execution id"})
			return
		}
		if strings.HasSuffix(path, "/cancel") {
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
	mux.HandleFunc("/api/v1/audit", withAuth(db, handleSearchAudit))
	mux.HandleFunc("/api/v1/audit/verify", withAuth(db, handleVerifyAudit))
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
	mux.HandleFunc("/api/v1/jobs/renew", func(w http.ResponseWriter, r *http.Request) {
		handleRenewLease(r.Context(), db, w, r)
	})
	mux.HandleFunc("/api/v1/runbooks", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleListRunbooks(w, r)
			return
		}
		handleCreateRunbook(w, r)
	}))
	mux.HandleFunc("/api/v1/runbooks/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/runbooks/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing runbook name"})
			return
		}
		if strings.HasSuffix(path, "/run") {
			name := path[:len(path)-len("/run")]
			if r.Method == http.MethodPost {
				handleRunRunbook(w, r, name)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if strings.HasSuffix(path, "/publish") {
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
		if r.Method == http.MethodGet {
			handleListApprovals(w, r)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))
	mux.HandleFunc("/api/v1/approvals/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/approvals/"):]
		if strings.HasSuffix(path, "/approve") {
			approvalID := path[:len(path)-len("/approve")]
			if r.Method == http.MethodPost {
				handleApprove(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if strings.HasSuffix(path, "/deny") {
			approvalID := path[:len(path)-len("/deny")]
			if r.Method == http.MethodPost {
				handleDeny(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetApproval(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))
	mux.HandleFunc("/api/v1/schedules", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handleListSchedules(w, r)
			return
		}
		handleCreateSchedule(w, r)
	}))
	mux.HandleFunc("/api/v1/automation/status", withAuth(db, handleAutomationStatus))
	mux.HandleFunc("/api/v1/automation/pause", withAuth(db, handlePauseAutomation))
	mux.HandleFunc("/api/v1/automation/resume", withAuth(db, handleResumeAutomation))
	mux.HandleFunc("/api/v1/schedules/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		handleDisableSchedule(w, r, strings.TrimPrefix(r.URL.Path, "/api/v1/schedules/"))
	}))
	return db, mux, func() { db.Close() }
}

func TestHeaderIdentityIsRejectedWhenDevAuthDisabled(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_DEV_AUTH", "false")

	h := withAuth(db, handleWhoAmI)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	r.Header.Set("X-VPS-User", "user_senior")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected header identity to be rejected, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductionModeRejectsDevIdentityAndRunnerBypass(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_ENV", "production")
	t.Setenv("VPS_DEV_AUTH", "true")

	userRequest := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	userRequest.Header.Set("X-VPS-User", "user_senior")
	userResponse := httptest.NewRecorder()
	withAuth(db, handleWhoAmI)(userResponse, userRequest)
	if userResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected production to reject X-VPS-User, got %d", userResponse.Code)
	}

	runnerRequest := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	if _, err := authenticateRunnerRegistration(db, runnerRequest); err == nil {
		t.Fatal("expected production to reject the dev runner bypass")
	}
}

func TestProductionAuthConfigFailsClosed(t *testing.T) {
	t.Setenv("VPS_ENV", "production")
	t.Setenv("VPS_DEV_AUTH", "true")
	if err := validateAuthConfig(); err == nil {
		t.Fatal("expected production development auth configuration to fail")
	}
	t.Setenv("VPS_DEV_AUTH", "false")
	t.Setenv("VPS_WEB_SHARED_SECRET", "short")
	if err := validateAuthConfig(); err == nil {
		t.Fatal("expected short production shared secret to fail")
	}
	t.Setenv("VPS_WEB_SHARED_SECRET", strings.Repeat("s", 32))
	if err := validateAuthConfig(); err != nil {
		t.Fatalf("expected valid production auth configuration, got %v", err)
	}
}

func TestExternalIdentityCannotRebindProvisionedSubject(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	if _, err := db.Exec(`UPDATE users SET external_subject = 'subject-existing', external_provider = 'zitadel' WHERE id = 'user_senior'`); err != nil {
		t.Fatalf("bind existing subject: %v", err)
	}
	if _, err := resolveExternalActor(context.Background(), db, "subject-attacker", "senior@example.com"); err == nil {
		t.Fatal("expected email fallback to reject a user already bound to another subject")
	}
}

func TestExternalIdentityRequiresMatchingSharedSecret(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_DEV_AUTH", "false")
	t.Setenv("VPS_WEB_SHARED_SECRET", strings.Repeat("s", 32))

	h := withAuth(db, handleWhoAmI)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	r.Header.Set("X-VPS-Internal-Secret", "wrong")
	r.Header.Set("X-VPS-OIDC-Subject", "subject-senior")
	r.Header.Set("X-VPS-OIDC-Email", "senior@example.com")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected mismatched shared secret to be rejected, got %d", w.Code)
	}
}

func TestBearerTokenAuthenticatesProductionAPI(t *testing.T) {
	db, _, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_DEV_AUTH", "false")
	t.Setenv("VPS_ENV", "production")
	t.Setenv("VPS_WEB_SHARED_SECRET", strings.Repeat("s", 32))
	if _, err := db.Exec(`INSERT INTO api_tokens (id, organisation_id, user_id, name, token_prefix, token_hash, expires_at) VALUES ('pat_test','org_demo','user_senior','test','api-test',?,datetime('now','+1 hour'))`, hashToken("api-test-token")); err != nil {
		t.Fatalf("insert API token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	req.Header.Set("Authorization", "Bearer api-test-token")
	res := httptest.NewRecorder()
	withAuth(db, handleWhoAmI)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected bearer token authentication, got %d: %s", res.Code, res.Body.String())
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/v1/whoami", nil)
	bad.Header.Set("Authorization", "Bearer wrong-token")
	badRes := httptest.NewRecorder()
	withAuth(db, handleWhoAmI)(badRes, bad)
	if badRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid bearer token rejection, got %d", badRes.Code)
	}
}

func doRequest(t *testing.T, mux *http.ServeMux, method, path, body, user string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	// The API never infers an identity, so the test helper supplies the
	// default actor explicitly rather than relying on server-side behaviour.
	if user == "" {
		user = "user_senior"
	}
	req.Header.Set("X-VPS-User", user)
	if strings.HasPrefix(path, "/api/v1/jobs/") {
		req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestSeniorExecDevAllowed(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo hello"}`, "user_senior")
	if w.Code != 201 {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateExecutionIdempotencyReplaysAndRejectsPayloadConflicts(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	submit := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/executions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-VPS-User", "user_senior")
		req.Header.Set("Idempotency-Key", "deploy-20260727-01")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	first := submit(`{"target":"server:srv_demo","command":"echo idempotent","reason":"release"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first request failed: %d %s", first.Code, first.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &firstBody); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	second := submit(`{"target":"server:srv_demo","command":"echo idempotent","reason":"release"}`)
	if second.Code != http.StatusCreated || second.Header().Get("Idempotency-Replayed") != "true" || second.Body.String() != first.Body.String() {
		t.Fatalf("identical retry should replay original response: first=%q second=%q replay=%q", first.Body.String(), second.Body.String(), second.Header().Get("Idempotency-Replayed"))
	}
	conflict := submit(`{"target":"server:srv_demo","command":"echo changed","reason":"release"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "different request payload") {
		t.Fatalf("changed retry should be rejected: %d %s", conflict.Code, conflict.Body.String())
	}
	list := doRequest(t, mux, http.MethodGet, "/api/v1/executions", "", "user_senior")
	if !strings.Contains(list.Body.String(), firstBody["execution_id"].(string)) {
		t.Fatalf("original execution should remain listed: %s", list.Body.String())
	}
}

func TestRunbookIdempotencyReplaysSubmittedExecution(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	submit := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/runbooks/check-uptime/run", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-VPS-User", "user_senior")
		req.Header.Set("Idempotency-Key", "runbook-change-01")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	first := submit(`{"target":"server:srv_demo","reason":"nightly health"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("first runbook request failed: %d %s", first.Code, first.Body.String())
	}
	second := submit(`{"target":"server:srv_demo","reason":"nightly health"}`)
	if second.Code != http.StatusCreated || second.Header().Get("Idempotency-Replayed") != "true" || second.Body.String() != first.Body.String() {
		t.Fatalf("identical runbook retry should replay: first=%q second=%q replay=%q", first.Body.String(), second.Body.String(), second.Header().Get("Idempotency-Replayed"))
	}
	conflict := submit(`{"target":"server:srv_demo","reason":"changed reason"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "different request payload") {
		t.Fatalf("changed runbook retry should be rejected: %d %s", conflict.Code, conflict.Body.String())
	}
}

func TestJuniorExecRawDenied(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo hello"}`, "user_junior")
	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListExecutionsDoesNotReenterSingleSQLiteConnection(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions", `{"target":"server:srv_demo","command":"echo smoke"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("create execution failed: %d %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodGet, "/api/v1/executions", "", "user_senior")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "executions") {
		t.Fatalf("list executions failed: %d %s", w.Code, w.Body.String())
	}
}

func TestCancelExecutionEnforcesRequesterOrSeniorRole(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	insertExecution := func(t *testing.T, id, userID, role string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, execution_type, status, risk_level, command) VALUES (?, 'org_demo', ?, ?, 'raw_command', 'queued', 'low', 'uptime')`, id, userID, role); err != nil {
			t.Fatalf("insert execution: %v", err)
		}
	}
	insertExecution(t, "exec_cancel_junior", "user_junior", "junior_engineer")
	insertExecution(t, "exec_cancel_senior", "user_senior", "senior_engineer")
	insertExecution(t, "exec_cancel_auditor", "user_senior", "senior_engineer")

	if w := doRequest(t, mux, http.MethodPost, "/api/v1/executions/exec_cancel_junior/cancel", "", "user_junior"); w.Code != http.StatusOK {
		t.Fatalf("requester should be able to cancel own execution, got %d: %s", w.Code, w.Body.String())
	}
	if w := doRequest(t, mux, http.MethodPost, "/api/v1/executions/exec_cancel_senior/cancel", "", "user_junior"); w.Code != http.StatusForbidden {
		t.Fatalf("junior should not cancel another user's execution, got %d: %s", w.Code, w.Body.String())
	}
	if w := doRequest(t, mux, http.MethodPost, "/api/v1/executions/exec_cancel_auditor/cancel", "", "user_auditor"); w.Code != http.StatusForbidden {
		t.Fatalf("auditor should not cancel an execution, got %d: %s", w.Code, w.Body.String())
	}
	if w := doRequest(t, mux, http.MethodPost, "/api/v1/executions/exec_cancel_senior/cancel", "", "user_senior"); w.Code != http.StatusOK {
		t.Fatalf("senior should be able to cancel another user's execution, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditorExecDenied(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"uptime"}`, "user_auditor")
	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductionWithoutReasonDenied(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/servers",
		`{"name":"prod-srv","hostname":"prod.local","environment":"production"}`, "user_senior")

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:prod-srv","command":"uptime"}`, "user_senior")
	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProductionWithReasonAllowed(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/servers",
		`{"name":"prod-srv","hostname":"prod.local","environment":"production"}`, "user_senior")

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:prod-srv","command":"uptime","reason":"maintenance check"}`, "user_senior")
	if w.Code != 201 {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuditEventsOnExecLifecycle(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo test"}`, "user_senior")
	if w.Code != 201 {
		t.Fatalf("exec create failed: %s", w.Body.String())
	}

	w = doRequest(t, mux, http.MethodGet, "/api/v1/audit?limit=10", "", "user_senior")
	var resp struct{ Events []map[string]any }
	json.NewDecoder(w.Body).Decode(&resp)

	found := false
	for _, e := range resp.Events {
		if e["action"] == "execution.requested" && e["result"] == "queued" {
			found = true
		}
	}
	if !found {
		t.Error("audit event 'execution.requested' not found")
	}
}

func TestDenialCreatesAuditEvent(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo hello"}`, "user_junior")

	w := doRequest(t, mux, http.MethodGet, "/api/v1/audit?limit=10", "", "user_senior")
	var resp struct{ Events []map[string]any }
	json.NewDecoder(w.Body).Decode(&resp)

	found := false
	for _, e := range resp.Events {
		if e["action"] == "execution.requested" && e["result"] == "denied" {
			found = true
		}
	}
	if !found {
		t.Error("denial audit event not found")
	}
}

func TestAuditHashChainVerificationDetectsTampering(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/executions", `{"target":"server:srv_demo","command":"echo audit"}`, "user_senior")
	valid := doRequest(t, mux, http.MethodGet, "/api/v1/audit/verify", "", "user_senior")
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"valid":true`) {
		t.Fatalf("audit chain should verify before tampering: %d %s", valid.Code, valid.Body.String())
	}
	if _, err := db.Exec("UPDATE audit_events SET metadata = ? WHERE action = 'execution.requested'", `{"tampered":true}`); err != nil {
		t.Fatalf("tamper audit event: %v", err)
	}
	invalid := doRequest(t, mux, http.MethodGet, "/api/v1/audit/verify", "", "user_senior")
	if invalid.Code != http.StatusConflict || !strings.Contains(invalid.Body.String(), `"valid":false`) {
		t.Fatalf("audit chain should detect tampering: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestTenantIsolation(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/servers",
		`{"name":"my-srv","hostname":"my.local"}`, "user_senior")

	w := doRequest(t, mux, http.MethodGet, "/api/v1/servers", "", "user_senior")
	var resp struct{ Servers []struct{ ID string } }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Servers) < 2 {
		t.Error("expected at least 2 servers for senior")
	}

	w = doRequest(t, mux, http.MethodGet, "/api/v1/servers", "", "user_junior")
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Servers) < 2 {
		t.Error("junior should see same servers (same org)")
	}
}

func TestRunnerScopeClaimByOrg(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo test"}`, "user_senior")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?organisation_id=org_other", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code == 200 {
		var j struct{ ExecutionID string }
		json.NewDecoder(w2.Body).Decode(&j)
		if j.ExecutionID != "" {
			t.Error("cross-org job claim should not return a job")
		}
	}
}

func TestRedaction(t *testing.T) {
	input := "export API_KEY=sk-abc123def456\nresult: token=ghp_1234567890abcdef1234567890abcdef123456"
	output := redact.Stdout(input)
	if strings.Contains(output, "sk-abc123def456") {
		t.Error("API key not redacted")
	}
	if strings.Contains(output, "ghp_123456") {
		t.Error("GitHub token not redacted")
	}
	if !strings.Contains(output, "REDACTED") {
		t.Error("expected REDACTED in output")
	}
}

func TestApprovalPipelineCreatesExecution(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	// Senior creates a high-risk runbook for production
	doRequest(t, mux, http.MethodPost, "/api/v1/servers",
		`{"name":"prod-srv","hostname":"prod.local","environment":"production"}`, "user_senior")

	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks",
		`{"name":"restart-prod","title":"Restart Prod","command":"systemctl restart app","risk":"high","environment":"production","allowed_roles":"[\"senior_engineer\",\"admin\",\"owner\",\"junior_engineer\"]"}`, "user_senior")
	if w.Code != 201 {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}

	// Publish it
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/restart-prod/publish", "", "user_senior")
	if w.Code != 200 {
		t.Fatalf("runbook publish failed: %s", w.Body.String())
	}

	// Junior runs it on production - should require approval
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/restart-prod/run",
		`{"target":"server:prod-srv","reason":"deployment"}`, "user_junior")
	if w.Code != 202 {
		t.Fatalf("expected 202 awaiting approval, got %d: %s", w.Code, w.Body.String())
	}
	var approvalResp map[string]any
	json.NewDecoder(w.Body).Decode(&approvalResp)
	approvalID, ok := approvalResp["approval_id"].(string)
	if !ok || approvalID == "" {
		t.Fatal("approval_id missing in response")
	}

	// Senior approves - should auto-create execution
	w = doRequest(t, mux, http.MethodPost, "/api/v1/approvals/"+approvalID+"/approve", "", "user_senior")
	if w.Code != 200 {
		t.Fatalf("approve failed: %s", w.Body.String())
	}
	var approveResp map[string]string
	json.NewDecoder(w.Body).Decode(&approveResp)
	execID := approveResp["execution_id"]
	if execID == "" {
		t.Fatal("execution_id missing after approval")
	}

	// Verify execution has delegated_by_user_id set
	w = doRequest(t, mux, http.MethodGet, "/api/v1/executions/"+execID, "", "user_senior")
	var execResp struct {
		Execution struct {
			DelegatedBy string `json:"delegated_by_user_id"`
			ApprovalID  string `json:"approval_id"`
		} `json:"execution"`
	}
	json.NewDecoder(w.Body).Decode(&execResp)
	if execResp.Execution.DelegatedBy != "user_senior" {
		t.Errorf("expected delegated_by_user_id=user_senior, got %s", execResp.Execution.DelegatedBy)
	}
	if execResp.Execution.ApprovalID != approvalID {
		t.Errorf("expected approval_id=%s, got %s", approvalID, execResp.Execution.ApprovalID)
	}
	var requesterRole string
	if err := db.QueryRow("SELECT actor_role_at_time FROM executions WHERE id = ?", execID).Scan(&requesterRole); err != nil {
		t.Fatalf("read execution actor role: %v", err)
	}
	if requesterRole != "junior_engineer" {
		t.Fatalf("expected requester role to be preserved, got %q", requesterRole)
	}
}

func TestApprovalDenialRequiresNote(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	_, err := db.Exec(`INSERT INTO approval_requests (id, organisation_id, requester_user_id, action_type, status, risk_level, reason, target_type, target_id, target_snapshot, request_payload, expires_at, created_at)
		VALUES ('apr_note', 'org_demo', 'user_junior', 'runbook.execute', 'pending', 'high', 'test', 'server', 'srv_demo', '{}', '{}', datetime('now','+1 hour'), datetime('now'))`)
	if err != nil {
		t.Fatalf("insert approval: %v", err)
	}
	w := doRequest(t, mux, http.MethodPost, "/api/v1/approvals/apr_note/deny", `{}`, "user_senior")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected missing denial note to return 400, got %d: %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/approvals/apr_note/deny", `{"note":"   "}`, "user_senior")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected whitespace denial note to return 400, got %d: %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/approvals/apr_note/deny", `{"note":"needs a safer rollback"}`, "user_senior")
	if w.Code != http.StatusOK {
		t.Fatalf("expected valid denial note to succeed, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExpiredApprovalCannotBeDenied(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	_, err := db.Exec(`INSERT INTO approval_requests (id, organisation_id, requester_user_id, action_type, status, risk_level, reason, target_type, target_id, target_snapshot, request_payload, expires_at, created_at)
		VALUES ('apr_expired', 'org_demo', 'user_junior', 'runbook.execute', 'pending', 'high', 'test', 'server', 'srv_demo', '{}', '{}', datetime('now','-1 minute'), datetime('now','-2 minutes'))`)
	if err != nil {
		t.Fatalf("insert expired approval: %v", err)
	}
	w := doRequest(t, mux, http.MethodPost, "/api/v1/approvals/apr_expired/deny", `{"note":"expired"}`, "user_senior")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected expired denial to return 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRunbookScopingDeniesJunior(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	// Senior creates a runbook scoped to seniors only
	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks",
		`{"name":"senior-only","title":"Senior Only","command":"whoami","allowed_roles":"[\"senior_engineer\",\"admin\",\"owner\"]"}`, "user_senior")
	if w.Code != 201 {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/senior-only/publish", "", "user_senior")
	if w.Code != 200 {
		t.Fatalf("publish failed: %s", w.Body.String())
	}

	// Junior tries to run it - should be denied
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/senior-only/run",
		`{"target":"server:srv_demo"}`, "user_junior")
	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestStdoutStderrStoredOnSubmit(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	// Senior creates execution
	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo hello"}`, "user_senior")
	if w.Code != 201 {
		t.Fatalf("exec create failed: %s", w.Body.String())
	}
	var createResp struct {
		ExecutionID string `json:"execution_id"`
	}
	json.NewDecoder(w.Body).Decode(&createResp)

	// Runner claims job
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req)
	if w2.Code != 200 {
		t.Fatalf("claim job failed: %s", w2.Body.String())
	}

	// Runner submits result with stdout/stderr
	var claimResp struct {
		TargetID string `json:"target_id"`
		LeaseID  string `json:"lease_id"`
	}
	json.NewDecoder(w2.Body).Decode(&claimResp)
	renewReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/renew", strings.NewReader(fmt.Sprintf(`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s"}`, claimResp.TargetID, createResp.ExecutionID, claimResp.LeaseID)))
	renewReq.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	renewResp := httptest.NewRecorder()
	mux.ServeHTTP(renewResp, renewReq)
	if renewResp.Code != http.StatusOK {
		t.Fatalf("lease renewal failed: %d %s", renewResp.Code, renewResp.Body.String())
	}
	w3 := doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result",
		fmt.Sprintf(`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s","exit_code":0,"stdout":"hello output\n","stderr":"","duration_ms":100}`, claimResp.TargetID, createResp.ExecutionID, claimResp.LeaseID), "")
	if w3.Code != 200 {
		t.Fatalf("submit result failed: %s", w3.Body.String())
	}
	w3 = doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result",
		fmt.Sprintf(`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s","exit_code":0,"stdout":"hello output\n","stderr":"","duration_ms":100}`, claimResp.TargetID, createResp.ExecutionID, claimResp.LeaseID), "")
	if w3.Code != http.StatusOK || !strings.Contains(w3.Body.String(), `"status":"ok"`) {
		t.Fatalf("replayed result should be idempotent, got %d: %s", w3.Code, w3.Body.String())
	}
	w3 = doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result",
		fmt.Sprintf(`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s","exit_code":0,"stdout":"tampered output\n","stderr":"","duration_ms":100}`, claimResp.TargetID, createResp.ExecutionID, claimResp.LeaseID), "")
	if w3.Code != http.StatusConflict {
		t.Fatalf("replayed result with a different payload should be rejected, got %d: %s", w3.Code, w3.Body.String())
	}

	// Verify stdout is stored
	w4 := doRequest(t, mux, http.MethodGet, "/api/v1/executions/"+createResp.ExecutionID, "", "user_senior")
	bodyStr := w4.Body.String()
	var execResp struct {
		Targets []struct {
			Stdout string `json:"stdout"`
			Stderr string `json:"stderr"`
		} `json:"targets"`
	}
	json.NewDecoder(w4.Body).Decode(&execResp)
	if len(execResp.Targets) == 0 {
		t.Fatalf("no targets in execution. response: %s", bodyStr)
	}
	if !strings.Contains(execResp.Targets[0].Stdout, "hello output") {
		t.Errorf("stdout not stored: got %q", execResp.Targets[0].Stdout)
	}
}

func TestRunnerFailureUsesRetryBudgetAndDeadLetters(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"false"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("exec create failed: %s", w.Body.String())
	}
	var created struct {
		ExecutionID string `json:"execution_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	claim := func() struct{ TargetID, LeaseID string } {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
		req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("claim failed: %s", response.Body.String())
		}
		var result struct {
			TargetID string
			LeaseID  string
		}
		var body struct {
			TargetID string `json:"target_id"`
			LeaseID  string `json:"lease_id"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode claim response: %v", err)
		}
		result.TargetID = body.TargetID
		result.LeaseID = body.LeaseID
		return result
	}

	submitFailure := func(targetID, leaseID string) *httptest.ResponseRecorder {
		return doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result", fmt.Sprintf(
			`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s","exit_code":1,"error":"command failed"}`,
			targetID, created.ExecutionID, leaseID), "")
	}

	first := claim()
	if response := submitFailure(first.TargetID, first.LeaseID); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "retry_scheduled") {
		t.Fatalf("first failure should schedule a retry, got %d: %s", response.Code, response.Body.String())
	}
	var status, targetStatus string
	if err := db.QueryRow("SELECT status FROM executions WHERE id = ?", created.ExecutionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("execution status after retry = %q, want running", status)
	}
	if _, err := db.Exec("UPDATE execution_targets SET next_attempt_at = datetime('now','-1 second') WHERE execution_id = ?", created.ExecutionID); err != nil {
		t.Fatal(err)
	}

	second := claim()
	if _, err := db.Exec("UPDATE execution_targets SET max_attempts = attempt WHERE id = ?", second.TargetID); err != nil {
		t.Fatal(err)
	}
	if response := submitFailure(second.TargetID, second.LeaseID); response.Code != http.StatusOK {
		t.Fatalf("final failure should be accepted, got %d: %s", response.Code, response.Body.String())
	}
	if err := db.QueryRow("SELECT status FROM execution_targets WHERE id = ?", second.TargetID).Scan(&targetStatus); err != nil {
		t.Fatal(err)
	}
	if targetStatus != "dead_letter" {
		t.Fatalf("target status = %q, want dead_letter", targetStatus)
	}
	if err := db.QueryRow("SELECT status FROM executions WHERE id = ?", created.ExecutionID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("execution status = %q, want failed", status)
	}
}

func TestExpiredLeaseAtAttemptLimitIsReconciled(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"uptime"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("exec create failed: %s", w.Body.String())
	}
	var created struct {
		ExecutionID string `json:"execution_id"`
	}
	json.NewDecoder(w.Body).Decode(&created)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	claimResponse := httptest.NewRecorder()
	mux.ServeHTTP(claimResponse, req)
	if claimResponse.Code != http.StatusOK {
		t.Fatalf("claim failed: %s", claimResponse.Body.String())
	}
	var claim struct {
		TargetID string `json:"target_id"`
	}
	json.NewDecoder(claimResponse.Body).Decode(&claim)
	if _, err := db.Exec(`UPDATE execution_targets SET attempt = max_attempts, lease_expires_at = datetime('now','-1 second') WHERE id = ?`, claim.TargetID); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	req.Header.Set("X-VPS-Runner-Token", "test-runner-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != http.StatusNotFound {
		t.Fatalf("reconciliation should leave no claimable job, got %d: %s", response.Code, response.Body.String())
	}
	var targetStatus, executionStatus string
	db.QueryRow("SELECT status FROM execution_targets WHERE id = ?", claim.TargetID).Scan(&targetStatus)
	db.QueryRow("SELECT status FROM executions WHERE id = ?", created.ExecutionID).Scan(&executionStatus)
	if targetStatus != "dead_letter" || executionStatus != "failed" {
		t.Fatalf("reconciled states = target %q, execution %q, want dead_letter, failed", targetStatus, executionStatus)
	}
}

func TestRunnerJobsRequireCredential(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()
	t.Setenv("VPS_DEV_AUTH", "false")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated runner claim to return 401, got %d", w.Code)
	}
}

func TestRunnerCredentialRotationBindsAndRevokes(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	runner := struct{ ID string }{ID: "rnr_local"}

	issue := func() string {
		response := doRequest(t, mux, http.MethodPost, "/api/v1/runners/registration-token", fmt.Sprintf(`{"runner_id":%q}`, runner.ID), "user_senior")
		if response.Code != http.StatusCreated {
			t.Fatalf("token issue failed: %s", response.Body.String())
		}
		var body struct {
			Token string `json:"registration_token"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode token: %v", err)
		}
		return body.Token
	}
	token1 := issue()
	token2 := issue()
	var revoked int
	if err := db.QueryRow("SELECT COUNT(*) FROM runner_credentials WHERE token_hash = ? AND revoked_at IS NOT NULL", hashToken(token1)).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 1 {
		t.Fatalf("previous credential revoked count = %d, want 1", revoked)
	}

	register := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/runners", strings.NewReader(`{"name":"rotatable-runner","version":"test"}`))
		request = request.WithContext(context.WithValue(request.Context(), dbKey, db))
		request.Header.Set("X-VPS-Runner-Token", token)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		return response
	}
	if response := register(token1); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential registration status = %d, want 401", response.Code)
	}
	response := register(token2)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), runner.ID) {
		t.Fatalf("rotated credential did not re-register bound runner: %d %s", response.Code, response.Body.String())
	}

	revokedResponse := doRequest(t, mux, http.MethodDelete, "/api/v1/runners/"+runner.ID, "", "user_senior")
	if revokedResponse.Code != http.StatusOK {
		t.Fatalf("runner revoke failed: %d %s", revokedResponse.Code, revokedResponse.Body.String())
	}
	if response := register(token2); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked runner credential registration status = %d, want 401", response.Code)
	}
}

func TestRunnerCredentialIssuanceRequiresRunnerManager(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()
	response := doRequest(t, mux, http.MethodPost, "/api/v1/runners/registration-token", "", "user_junior")
	if response.Code != http.StatusForbidden {
		t.Fatalf("junior credential issuance status = %d, want 403", response.Code)
	}
}

func TestRunbookRejectsInvalidParametersBeforeQueueing(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks",
		`{"name":"service-check","title":"Service check","command":"systemctl is-active ${service}","risk":"low","environment":"development"}`, "user_senior")
	// The compact create form does not define parameters, so the placeholder is
	// rejected when the runbook is run rather than silently expanded by a shell.
	if w.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/service-check/publish", "", "user_senior")
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/service-check/run", `{"target":"server:srv_demo","params":{"service":"nginx; touch /tmp/pwned"}}`, "user_senior")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid runbook parameters to return 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRunbookRejectsMixedEnvironmentTargets(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/servers", `{"name":"staging-srv","hostname":"staging.local","environment":"staging","tags":[{"key":"role","value":"app"}]}`, "user_senior")
	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks", `{"name":"mixed-check","title":"Mixed check","command":"uptime","risk":"low","environment":"*","allowed_roles":"[\"senior_engineer\",\"admin\",\"owner\"]"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/mixed-check/publish", "", "user_senior")
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/mixed-check/run", `{"target":"tag:role=app"}`, "user_senior")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected mixed environments to return 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestApprovalStoresTargetSnapshot(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	doRequest(t, mux, http.MethodPost, "/api/v1/servers", `{"name":"prod-srv","hostname":"prod.local","environment":"production"}`, "user_senior")
	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks", `{"name":"restart-service","title":"Restart service","command":"systemctl restart app","risk":"high","environment":"production","allowed_roles":"[\"junior_engineer\",\"senior_engineer\",\"admin\",\"owner\"]"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/restart-service/publish", "", "user_senior")
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/restart-service/run", `{"target":"server:prod-srv","reason":"approved maintenance"}`, "user_junior")
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected approval request, got %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		ApprovalID string `json:"approval_id"`
	}
	json.NewDecoder(w.Body).Decode(&response)
	w = doRequest(t, mux, http.MethodGet, "/api/v1/approvals/"+response.ApprovalID, "", "user_senior")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "prod-srv") {
		t.Fatalf("approval target snapshot missing: %d %s", w.Code, w.Body.String())
	}
}

func TestRunbookPreflightDoesNotQueueExecution(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks", `{"name":"preflight-check","title":"Preflight check","command":"uptime","risk":"low","environment":"development"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/preflight-check/publish", "", "user_senior")
	w = doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/preflight-check/run", `{"target":"server:srv_demo","dry_run":true}`, "user_senior")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"preflight"`) {
		t.Fatalf("expected preflight response, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM executions WHERE organisation_id = 'org_demo'").Scan(&count); err != nil {
		t.Fatalf("count executions: %v", err)
	}
	if count != 0 {
		t.Fatalf("preflight queued %d executions", count)
	}
}

func TestEmbeddedSchedulerQueuesSafeRunbook(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	w := doRequest(t, mux, http.MethodPost, "/api/v1/schedules", fmt.Sprintf(`{"name":"uptime-every-minute","runbook_name":"check-uptime","target":"server:srv_demo","reason":"routine health check","interval_seconds":60,"next_run_at":%q}`, past), "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("schedule create failed: %d %s", w.Code, w.Body.String())
	}
	if err := runDueSchedulesOnce(context.Background(), db); err != nil {
		t.Fatalf("scheduler cycle failed: %v", err)
	}
	var executionID, actorRole, commandPreview, command string
	if err := db.QueryRow("SELECT id, actor_role_at_time, command_preview, command FROM executions WHERE execution_type = 'runbook' ORDER BY requested_at DESC LIMIT 1").Scan(&executionID, &actorRole, &commandPreview, &command); err != nil {
		t.Fatalf("scheduled execution missing: %v", err)
	}
	if executionID == "" || actorRole != "automation" || commandPreview != "uptime" || command != "uptime" {
		t.Fatalf("unexpected scheduled execution: id=%s role=%s preview=%s command=%s", executionID, actorRole, commandPreview, command)
	}
	var actorType string
	if err := db.QueryRow("SELECT actor_type FROM audit_events WHERE action = 'automation.execution.queued' ORDER BY occurred_at DESC LIMIT 1").Scan(&actorType); err != nil {
		t.Fatalf("automation audit event missing: %v", err)
	}
	if actorType != "automation" {
		t.Fatalf("expected automation audit actor, got %q", actorType)
	}
}

func TestScheduleCRUDRequiresSeniorAndCanBeDisabled(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	past := time.Now().UTC().Add(time.Minute).Format(time.RFC3339)
	w := doRequest(t, mux, http.MethodPost, "/api/v1/schedules", fmt.Sprintf(`{"name":"junior-schedule","runbook_name":"check-uptime","target":"server:srv_demo","reason":"routine","interval_seconds":60,"next_run_at":%q}`, past), "user_junior")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected junior schedule creation to be denied, got %d: %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/schedules", fmt.Sprintf(`{"name":"disable-me","runbook_name":"check-uptime","target":"server:srv_demo","reason":"routine","interval_seconds":60,"next_run_at":%q}`, past), "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("schedule creation failed: %d %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodGet, "/api/v1/schedules", "", "user_senior")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "disable-me") {
		t.Fatalf("schedule list failed: %d %s", w.Code, w.Body.String())
	}
	var scheduleID string
	if err := db.QueryRow("SELECT id FROM automation_schedules WHERE name = 'disable-me'").Scan(&scheduleID); err != nil {
		t.Fatalf("schedule lookup failed: %v", err)
	}
	w = doRequest(t, mux, http.MethodDelete, "/api/v1/schedules/"+scheduleID, "", "user_senior")
	if w.Code != http.StatusOK {
		t.Fatalf("schedule disable failed: %d %s", w.Code, w.Body.String())
	}
}

func TestSchedulerDoesNotAutoExecuteHighRiskRunbook(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	server := doRequest(t, mux, http.MethodPost, "/api/v1/servers", `{"name":"prod-srv","hostname":"prod.local","environment":"production"}`, "user_senior")
	if server.Code != http.StatusCreated {
		t.Fatalf("server create failed: %s", server.Body.String())
	}
	w := doRequest(t, mux, http.MethodPost, "/api/v1/runbooks", `{"name":"dangerous-check","title":"Dangerous check","command":"systemctl restart app","risk":"high","environment":"production"}`, "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("runbook create failed: %s", w.Body.String())
	}
	doRequest(t, mux, http.MethodPost, "/api/v1/runbooks/dangerous-check/publish", "", "user_senior")
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	w = doRequest(t, mux, http.MethodPost, "/api/v1/schedules", fmt.Sprintf(`{"name":"dangerous-schedule","runbook_name":"dangerous-check","target":"server:prod-srv","reason":"maintenance","interval_seconds":60,"next_run_at":%q}`, past), "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("schedule create failed: %s", w.Body.String())
	}
	if err := runDueSchedulesOnce(context.Background(), db); err != nil {
		t.Fatalf("scheduler cycle failed: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM executions WHERE execution_type = 'runbook' AND command_preview LIKE 'systemctl restart%'").Scan(&count); err != nil {
		t.Fatalf("count scheduled high-risk executions: %v", err)
	}
	if count != 0 {
		t.Fatalf("high-risk schedule queued %d executions", count)
	}
}

func TestAutomationPauseStopsAndResumesScheduledWork(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()

	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	w := doRequest(t, mux, http.MethodPost, "/api/v1/schedules", fmt.Sprintf(`{"name":"paused-schedule","runbook_name":"check-uptime","target":"server:srv_demo","reason":"routine","interval_seconds":60,"next_run_at":%q}`, past), "user_senior")
	if w.Code != http.StatusCreated {
		t.Fatalf("schedule create failed: %d %s", w.Code, w.Body.String())
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/automation/pause", `{"reason":"incident response"}`, "user_senior")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"paused":true`) {
		t.Fatalf("pause failed: %d %s", w.Code, w.Body.String())
	}
	if err := runDueSchedulesOnce(context.Background(), db); err != nil {
		t.Fatalf("paused scheduler cycle failed: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM executions WHERE execution_type = 'runbook'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("paused scheduler queued %d executions", count)
	}
	w = doRequest(t, mux, http.MethodPost, "/api/v1/automation/resume", "", "user_senior")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"paused":true`) {
		t.Fatalf("resume failed: %d %s", w.Code, w.Body.String())
	}
	if err := runDueSchedulesOnce(context.Background(), db); err != nil {
		t.Fatalf("resumed scheduler cycle failed: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM executions WHERE execution_type = 'runbook'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("resumed scheduler queued %d executions, want 1", count)
	}
}
