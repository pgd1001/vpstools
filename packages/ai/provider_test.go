package ai

import (
	"context"
	"strings"
	"testing"
)

type recordingProvider struct {
	request Request
}

func (p *recordingProvider) Complete(_ context.Context, request Request) (Response, error) {
	p.request = request
	return Response{Text: request.UserPrompt, Evidence: request.Evidence}, nil
}

func TestRedactingProviderProtectsPromptEvidenceAndResponse(t *testing.T) {
	inner := &recordingProvider{}
	provider := RedactingProvider{Inner: inner, Redact: func(value string) string {
		return strings.ReplaceAll(value, "token=secret", "token=[REDACTED]")
	}}
	response, err := provider.Complete(context.Background(), Request{
		SystemPrompt: "token=secret",
		UserPrompt:   "check token=secret",
		Evidence:     []Evidence{{Title: "secret", Content: "token=secret"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if inner.request.UserPrompt != "check token=[REDACTED]" || response.Text != "check token=[REDACTED]" {
		t.Fatalf("redaction did not apply: request=%q response=%q", inner.request.UserPrompt, response.Text)
	}
	if response.Evidence[0].Content != "token=[REDACTED]" {
		t.Fatalf("evidence was not redacted: %q", response.Evidence[0].Content)
	}
}
