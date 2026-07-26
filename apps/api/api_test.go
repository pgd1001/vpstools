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

	"github.com/pgd1001/svrtools/packages/redact"
	_ "modernc.org/sqlite"
)

func testAPI(t *testing.T) (*sql.DB, *http.ServeMux, func()) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
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
	if _, err := db.Exec(`INSERT INTO runner_credentials (id, organisation_id, token_hash, expires_at) VALUES ('rct_test','org_demo',?,datetime('now','+1 hour'))`, hashToken("test-runner-token")); err != nil {
		db.Close()
		t.Fatalf("runner credential: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/v1/whoami", withAuth(db, handleWhoAmI))
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
	mux.HandleFunc("/api/v1/schedules/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		handleDisableSchedule(w, r, strings.TrimPrefix(r.URL.Path, "/api/v1/schedules/"))
	}))
	return db, mux, func() { db.Close() }
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
	if user != "" {
		req.Header.Set("X-VPS-User", user)
	}
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

func TestJuniorExecRawDenied(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	w := doRequest(t, mux, http.MethodPost, "/api/v1/executions",
		`{"target":"server:srv_demo","command":"echo hello"}`, "user_junior")
	if w.Code != 403 {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
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
	_, mux, cleanup := testAPI(t)
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
	w3 := doRequest(t, mux, http.MethodPost, "/api/v1/jobs/result",
		fmt.Sprintf(`{"runner_id":"rnr_local","target_id":"%s","execution_id":"%s","lease_id":"%s","exit_code":0,"stdout":"hello output\n","stderr":"","duration_ms":100}`, claimResp.TargetID, createResp.ExecutionID, claimResp.LeaseID), "")
	if w3.Code != 200 {
		t.Fatalf("submit result failed: %s", w3.Body.String())
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

func TestRunnerJobsRequireCredential(t *testing.T) {
	_, mux, cleanup := testAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?runner_id=rnr_local", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated runner claim to return 401, got %d", w.Code)
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
