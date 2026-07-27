package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPProviderCompletesWithEvidenceAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("authorization header missing")
		}
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "local-model" || len(request.Messages) != 3 || !strings.Contains(request.Messages[1].Content, "uptime output") {
			t.Fatalf("unexpected request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatCompletionResponse{ID: "req-1", Model: "local-model", Choices: []struct {
			Message chatMessage `json:"message"`
		}{{Message: chatMessage{Role: "assistant", Content: "restart is safe"}}}})
	}))
	defer server.Close()
	provider := HTTPProvider{Endpoint: server.URL + "/v1", APIKey: "test-key", Model: "local-model"}
	response, err := provider.Complete(context.Background(), Request{SystemPrompt: "Be concise", UserPrompt: "Assess this", Evidence: []Evidence{{Kind: "execution", Title: "output", Content: "uptime output"}}})
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "restart is safe" || response.RequestID != "req-1" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestHTTPProviderFailsClosedOnInvalidConfigurationAndOversizedResponse(t *testing.T) {
	if _, err := (HTTPProvider{}).Complete(context.Background(), Request{UserPrompt: "test"}); err == nil {
		t.Fatal("expected missing endpoint error")
	}
	if _, err := (HTTPProvider{Endpoint: "file:///tmp/model", Model: "test"}).Complete(context.Background(), Request{UserPrompt: "test"}); err == nil {
		t.Fatal("expected invalid endpoint error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"large","model":"test","choices":[{"message":{"role":"assistant","content":"0123456789"}}]}`))
	}))
	defer server.Close()
	if _, err := (HTTPProvider{Endpoint: server.URL, Model: "test", MaxResponse: 8}).Complete(context.Background(), Request{UserPrompt: "test"}); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected response limit error, got %v", err)
	}
}
