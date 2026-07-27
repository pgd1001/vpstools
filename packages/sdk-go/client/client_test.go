package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestApprovalClientEscapesIdentifiersAndFilters(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.RequestURI)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(ListApprovalsResponse{})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	c := New(server.URL)
	if _, err := c.ListApprovals("pending review"); err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if _, err := c.ApproveApproval("apr test", "reviewed"); err != nil {
		t.Fatalf("approve approval: %v", err)
	}
	if _, err := c.DenyApproval("apr test", "not safe"); err != nil {
		t.Fatalf("deny approval: %v", err)
	}

	expected := []string{
		"GET /api/v1/approvals?status=pending+review",
		"POST /api/v1/approvals/apr%20test/approve",
		"POST /api/v1/approvals/apr%20test/deny",
	}
	if len(requests) != len(expected) {
		t.Fatalf("request count = %d, want %d: %v", len(requests), len(expected), requests)
	}
	for i := range expected {
		if requests[i] != expected[i] {
			t.Fatalf("request %d = %q, want %q", i, requests[i], expected[i])
		}
	}
}

func TestHealthAndReadyUseTypedEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/health":
			_, _ = w.Write([]byte(`{"status":"ok","database":"ok","version":"0.4.0","deployment_tier":"self-contained"}`))
		case "/api/v1/ready":
			_, _ = w.Write([]byte(`{"status":"ready","database":"ok","artifacts":"ok"}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL)
	health, err := c.Health()
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if health.Status != "ok" || health.DeploymentTier != "self-contained" {
		t.Fatalf("unexpected health response: %+v", health)
	}
	ready, err := c.Ready()
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	if ready.Status != "ready" || ready.Artifacts != "ok" {
		t.Fatalf("unexpected ready response: %+v", ready)
	}
}

func TestCreateExecutionWithIdempotencyKeySetsHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "change-01" {
			t.Errorf("Idempotency-Key = %q, want change-01", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"execution_id":"exe_test","status":"queued","target_count":1}`))
	}))
	defer server.Close()

	response, err := New(server.URL).CreateExecutionWithIdempotencyKey("server:srv_demo", "echo smoke", "release", "change-01")
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	if response.ExecutionID != "exe_test" || response.Status != "queued" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestRunRunbookWithIdempotencyKeySetsHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "run-01" {
			t.Errorf("Idempotency-Key = %q, want run-01", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"queued","execution_id":"exe_test"}`))
	}))
	defer server.Close()

	response, err := New(server.URL).RunRunbookWithIdempotencyKey("check-uptime", "server:srv_demo", "routine", nil, "run-01")
	if err != nil {
		t.Fatalf("run runbook: %v", err)
	}
	if response["execution_id"] != "exe_test" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestVerifyAuditRequestsVerificationEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/audit/verify" {
			t.Errorf("path = %q, want /api/v1/audit/verify", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"checked_events":4}`))
	}))
	defer server.Close()

	response, err := New(server.URL).VerifyAudit()
	if err != nil {
		t.Fatalf("verify audit: %v", err)
	}
	if !response.Valid || response.CheckedEvents != 4 {
		t.Fatalf("unexpected verification response: %+v", response)
	}
}
