package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pgd1001/svrtools/packages/redact"
	_ "modernc.org/sqlite"
)

func testAPI(t *testing.T) (*sql.DB, *http.ServeMux, func()) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	ctx := context.Background()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	if err := seed(ctx, db); err != nil {
		db.Close()
		t.Fatalf("seed: %v", err)
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
	mux.HandleFunc("/api/v1/audit", withAuth(db, handleSearchAudit))
	mux.HandleFunc("/api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		handleClaimJob(r.Context(), db, w, r)
	})
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/next?organisation_id=org_demo", nil)
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
