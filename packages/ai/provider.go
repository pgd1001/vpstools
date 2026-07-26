// Package ai defines the application boundary for model-assisted features.
// Providers may be local, managed, or hosted behind an organisation gateway.
package ai

import (
	"context"
	"errors"
)

// Evidence is a redacted, traceable item supplied to or returned by a model.
// The application stores identifiers and metadata separately from model text.
type Evidence struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	SourceURI string `json:"source_uri,omitempty"`
}

type Request struct {
	Model        string            `json:"model,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	UserPrompt   string            `json:"user_prompt"`
	Evidence     []Evidence        `json:"evidence,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type Response struct {
	Text      string         `json:"text"`
	Model     string         `json:"model,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Evidence  []Evidence     `json:"evidence,omitempty"`
	Usage     map[string]int `json:"usage,omitempty"`
}

// Provider is deliberately small so AI features don't depend on a vendor SDK.
type Provider interface {
	Complete(context.Context, Request) (Response, error)
}

// RedactingProvider applies the same secret-safety policy to prompts, evidence,
// and responses regardless of which provider is configured.
type RedactingProvider struct {
	Inner  Provider
	Redact func(string) string
}

func (p RedactingProvider) Complete(ctx context.Context, request Request) (Response, error) {
	if p.Inner == nil {
		return Response{}, errors.New("ai provider is not configured")
	}
	redact := p.Redact
	if redact == nil {
		redact = func(value string) string { return value }
	}
	request.SystemPrompt = redact(request.SystemPrompt)
	request.UserPrompt = redact(request.UserPrompt)
	for i := range request.Evidence {
		request.Evidence[i].Title = redact(request.Evidence[i].Title)
		request.Evidence[i].Content = redact(request.Evidence[i].Content)
	}
	response, err := p.Inner.Complete(ctx, request)
	if err != nil {
		return Response{}, err
	}
	response.Text = redact(response.Text)
	for i := range response.Evidence {
		response.Evidence[i].Title = redact(response.Evidence[i].Title)
		response.Evidence[i].Content = redact(response.Evidence[i].Content)
	}
	return response, nil
}
