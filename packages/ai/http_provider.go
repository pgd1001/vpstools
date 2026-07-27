package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPProvider speaks the OpenAI-compatible chat-completions protocol. It is
// suitable for managed gateways and local model servers that expose the same
// request shape. Authentication is optional so an in-process or local model
// can be used without an API key.
type HTTPProvider struct {
	Endpoint    string
	APIKey      string
	Model       string
	Timeout     time.Duration
	MaxResponse int64
	HTTPClient  *http.Client
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage map[string]int `json:"usage,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p HTTPProvider) Complete(ctx context.Context, request Request) (Response, error) {
	endpoint, err := p.endpoint()
	if err != nil {
		return Response{}, err
	}
	model := request.Model
	if model == "" {
		model = p.Model
	}
	if model == "" {
		return Response{}, errors.New("AI model is required")
	}
	messages := make([]chatMessage, 0, 2+len(request.Evidence))
	if request.SystemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: request.SystemPrompt})
	}
	for _, evidence := range request.Evidence {
		messages = append(messages, chatMessage{
			Role:    "user",
			Content: fmt.Sprintf("Evidence %s (%s):\n%s", evidence.Title, evidence.Kind, evidence.Content),
		})
	}
	messages = append(messages, chatMessage{Role: "user", Content: request.UserPrompt})
	payload, err := json.Marshal(chatCompletionRequest{Model: model, Messages: messages})
	if err != nil {
		return Response{}, fmt.Errorf("encode AI request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return Response{}, fmt.Errorf("create AI request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	client := p.HTTPClient
	if client == nil {
		timeout := p.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("AI request failed: %w", err)
	}
	defer response.Body.Close()
	maxResponse := p.MaxResponse
	if maxResponse == 0 {
		maxResponse = 4 << 20
	}
	if maxResponse < 1 || maxResponse > 64<<20 {
		return Response{}, errors.New("AI max response must be between 1 byte and 64 MiB")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return Response{}, fmt.Errorf("read AI response: %w", err)
	}
	if int64(len(body)) > maxResponse {
		return Response{}, errors.New("AI response exceeded configured limit")
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode AI response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := response.Status
		if decoded.Error != nil && decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
		return Response{}, fmt.Errorf("AI provider returned %s: %s", response.Status, message)
	}
	if len(decoded.Choices) == 0 || decoded.Choices[0].Message.Content == "" {
		return Response{}, errors.New("AI provider returned no completion")
	}
	return Response{Text: decoded.Choices[0].Message.Content, Model: decoded.Model, RequestID: decoded.ID, Usage: decoded.Usage}, nil
}

func (p HTTPProvider) endpoint() (string, error) {
	value := strings.TrimRight(strings.TrimSpace(p.Endpoint), "/")
	if value == "" {
		return "", errors.New("AI endpoint is required")
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("AI endpoint must be an absolute http or https URL")
	}
	if !strings.HasSuffix(u.Path, "/chat/completions") {
		u.Path = strings.TrimRight(u.Path, "/") + "/chat/completions"
	}
	return u.String(), nil
}
