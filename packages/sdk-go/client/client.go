package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

type WhoAmIResponse struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	Name           string `json:"name"`
	OrganisationID string `json:"organisation_id"`
	Organisation   string `json:"organisation"`
	Role           string `json:"role"`
}

func (c *Client) WhoAmI() (*WhoAmIResponse, error) {
	var resp WhoAmIResponse
	if err := c.get("/api/v1/whoami", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type Server struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Hostname    string            `json:"hostname"`
	Environment string            `json:"environment"`
	Tags        map[string]string `json:"tags"`
	Status      string            `json:"status"`
}

type ListServersResponse struct {
	Servers []Server `json:"servers"`
}

func (c *Client) ListServers() (*ListServersResponse, error) {
	var resp ListServersResponse
	if err := c.get("/api/v1/servers", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type CreateExecutionRequest struct {
	Target  string `json:"target"`
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type CreateExecutionResponse struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
}

func (c *Client) CreateExecution(target, command, reason string) (*CreateExecutionResponse, error) {
	body := CreateExecutionRequest{
		Target:  target,
		Command: command,
		Reason:  reason,
	}
	var resp CreateExecutionResponse
	if err := c.post("/api/v1/executions", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type AuditEvent struct {
	ID             string `json:"id"`
	OrganisationID string `json:"organisation_id"`
	ActorID        string `json:"actor_id"`
	Action         string `json:"action"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Result         string `json:"result"`
	MetadataJSON   string `json:"metadata_json"`
	CreatedAt      string `json:"created_at"`
}

type ListAuditResponse struct {
	Events []AuditEvent `json:"events"`
}

func (c *Client) ListAudit(limit string) (*ListAuditResponse, error) {
	var resp ListAuditResponse
	if err := c.get(fmt.Sprintf("/api/v1/audit?limit=%s", limit), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(path string, out any) error {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
