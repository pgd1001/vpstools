package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pgd1001/svrtools/packages/ai"
	"github.com/pgd1001/svrtools/packages/redact"
)

type testAIProvider struct{ request ai.Request }

func (p *testAIProvider) Complete(_ context.Context, request ai.Request) (ai.Response, error) {
	p.request = request
	return ai.Response{Text: "token=secret analysis", Model: "test-model", RequestID: "provider-1"}, nil
}

func TestAIAnalyzeRedactsPersistsAndAudits(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	provider := &testAIProvider{}
	apiAIProvider = ai.RedactingProvider{Inner: provider, Redact: redact.Stdout}
	defer func() { apiAIProvider = nil }()
	mux.HandleFunc("/api/v1/ai/analyze", withAuth(db, handleAIAnalyze))

	response := doRequest(t, mux, http.MethodPost, "/api/v1/ai/analyze", `{"question":"is this safe?","evidence":[{"kind":"command_output","title":"output","content":"token=secret"}]}`, "user_senior")
	if response.Code != http.StatusOK {
		t.Fatalf("analysis failed: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		AnalysisID string `json:"analysis_id"`
		Text       string `json:"text"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Text, "***REDACTED***") || strings.Contains(body.Text, "secret") {
		t.Fatalf("response was not redacted: %q", body.Text)
	}
	if strings.Contains(provider.request.Evidence[0].Content, "secret") {
		t.Fatalf("provider received a secret: %q", provider.request.Evidence[0].Content)
	}
	var stored, evidence, audits int
	if err := db.QueryRow("SELECT COUNT(*) FROM ai_requests WHERE id = ? AND response_text LIKE '%REDACTED%'", body.AnalysisID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM ai_evidence WHERE request_id = ? AND content LIKE '%REDACTED%'", body.AnalysisID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE action = 'ai.analysis.completed' AND target_id = ?", body.AnalysisID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if stored != 1 || evidence != 1 || audits != 1 {
		t.Fatalf("persistence counts request=%d evidence=%d audits=%d", stored, evidence, audits)
	}
}

func TestAIAnalyzeFailsClosedWhenUnconfigured(t *testing.T) {
	db, mux, cleanup := testAPI(t)
	defer cleanup()
	apiAIProvider = nil
	mux.HandleFunc("/api/v1/ai/analyze", withAuth(db, handleAIAnalyze))
	response := doRequest(t, mux, http.MethodPost, "/api/v1/ai/analyze", `{"question":"what happened?"}`, "user_senior")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}
