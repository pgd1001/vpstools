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

type ServerTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Server struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Hostname    string      `json:"hostname"`
	PublicIP    string      `json:"public_ip"`
	PrivateIP   string      `json:"private_ip"`
	SSHPort     int         `json:"ssh_port"`
	SSHUsername string      `json:"ssh_username"`
	Environment string      `json:"environment"`
	Provider    string      `json:"provider"`
	OSName      string      `json:"os_name"`
	OSVersion   string      `json:"os_version"`
	Kernel      string      `json:"kernel_version"`
	Arch        string      `json:"architecture"`
	Status      string      `json:"status"`
	LastSeenAt  string      `json:"last_seen_at"`
	LastCheckAt string      `json:"last_check_at"`
	CreatedAt   string      `json:"created_at"`
	Tags        []ServerTag `json:"tags"`
}

type ListServersResponse struct {
	Servers []Server `json:"servers"`
}

func (c *Client) ListServers(environment, tagKey, tagValue string) (*ListServersResponse, error) {
	path := "/api/v1/servers"
	sep := "?"
	if environment != "" {
		path += fmt.Sprintf("%senvironment=%s", sep, environment)
		sep = "&"
	}
	if tagKey != "" {
		path += fmt.Sprintf("%stag_key=%s", sep, tagKey)
		sep = "&"
	}
	if tagValue != "" {
		path += fmt.Sprintf("%stag_value=%s", sep, tagValue)
	}
	var resp ListServersResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type AddServerRequest struct {
	Name        string `json:"name"`
	Hostname    string `json:"hostname"`
	PublicIP    string `json:"public_ip"`
	PrivateIP   string `json:"private_ip"`
	SSHPort     int    `json:"ssh_port"`
	SSHUsername string `json:"ssh_username"`
	Environment string `json:"environment"`
	Provider    string `json:"provider"`
	Tags        []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"tags"`
}

type AddServerResponse struct {
	ServerID string `json:"server_id"`
	Status   string `json:"status"`
}

func (c *Client) AddServer(req AddServerRequest) (*AddServerResponse, error) {
	var resp AddServerResponse
	if err := c.post("/api/v1/servers", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type GetServerResponse struct {
	Server Server `json:"server"`
}

func (c *Client) GetServer(serverID string) (*GetServerResponse, error) {
	var resp GetServerResponse
	if err := c.get("/api/v1/servers/"+serverID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type CheckServerResponse struct {
	Server map[string]any `json:"server"`
}

func (c *Client) CheckServer(serverID string) (*CheckServerResponse, error) {
	var resp CheckServerResponse
	if err := c.post("/api/v1/servers/"+serverID+"/check", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type Runner struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RunnerType   string `json:"runner_type"`
	Status       string `json:"status"`
	Version      string `json:"version"`
	Hostname     string `json:"hostname"`
	Platform     string `json:"platform"`
	IPAddress    string `json:"ip_address"`
	LastSeenAt   string `json:"last_seen_at"`
	RegisteredAt string `json:"registered_at"`
	RevokedAt    string `json:"revoked_at"`
	CreatedAt    string `json:"created_at"`
}

type ListRunnersResponse struct {
	Runners []Runner `json:"runners"`
}

func (c *Client) ListRunners() (*ListRunnersResponse, error) {
	var resp ListRunnersResponse
	if err := c.get("/api/v1/runners", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type RegisterRunnerRequest struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Hostname   string `json:"hostname"`
	Platform   string `json:"platform"`
	IPAddress  string `json:"ip_address"`
	RunnerType string `json:"runner_type"`
}

type RegisterRunnerResponse struct {
	RunnerID string `json:"runner_id"`
	Status   string `json:"status"`
}

func (c *Client) RegisterRunner(req RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	var resp RegisterRunnerResponse
	if err := c.post("/api/v1/runners", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) RunnerHeartbeat(runnerID, hostname, platform, version string) error {
	body := map[string]string{
		"runner_id": runnerID,
		"hostname":  hostname,
		"platform":  platform,
		"version":   version,
	}
	var resp map[string]string
	return c.post("/api/v1/runners/heartbeat", body, &resp)
}

type CreateRegistrationTokenResponse struct {
	Token     string `json:"registration_token"`
	ExpiresIn string `json:"expires_in"`
}

func (c *Client) CreateRegistrationToken() (*CreateRegistrationTokenResponse, error) {
	var resp CreateRegistrationTokenResponse
	if err := c.post("/api/v1/runners/registration-token", nil, &resp); err != nil {
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
	TargetCount int    `json:"target_count"`
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

type ExecutionDetail struct {
	ID              string `json:"id"`
	ActorUserID     string `json:"actor_user_id"`
	ActorRole       string `json:"actor_role_at_time"`
	ExecutionType   string `json:"execution_type"`
	Status          string `json:"status"`
	RiskLevel       string `json:"risk_level"`
	Environment     string `json:"environment"`
	Reason          string `json:"reason"`
	CommandPreview  string `json:"command_preview"`
	CommandHash     string `json:"command_hash"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
	RequestedAt     string `json:"requested_at"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
	ErrorSummary    string `json:"error_summary"`
}

type ExecutionTarget struct {
	ID         string `json:"id"`
	ServerID   string `json:"server_id"`
	RunnerID   string `json:"runner_id"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Error      string `json:"error_summary"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

type GetExecutionResponse struct {
	Execution ExecutionDetail   `json:"execution"`
	Targets   []ExecutionTarget `json:"targets"`
}

func (c *Client) GetExecution(executionID string) (*GetExecutionResponse, error) {
	var resp GetExecutionResponse
	if err := c.get("/api/v1/executions/"+executionID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type ExecutionListItem struct {
	ID              string `json:"id"`
	ActorUserID     string `json:"actor_user_id"`
	ActorRole       string `json:"actor_role_at_time"`
	ExecutionType   string `json:"execution_type"`
	Status          string `json:"status"`
	CommandPreview  string `json:"command_preview"`
	TargetCount     int    `json:"target_count"`
	SucceededCount  int    `json:"succeeded_count"`
	FailedCount     int    `json:"failed_count"`
	RequestedAt     string `json:"requested_at"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at"`
}

type ListExecutionsResponse struct {
	Executions []ExecutionListItem `json:"executions"`
}

func (c *Client) ListExecutions(status, limit string) (*ListExecutionsResponse, error) {
	path := "/api/v1/executions"
	sep := "?"
	if status != "" {
		path += fmt.Sprintf("%sstatus=%s", sep, status)
		sep = "&"
	}
	if limit != "" {
		path += fmt.Sprintf("%slimit=%s", sep, limit)
	}
	var resp ListExecutionsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CancelExecution(executionID string) (map[string]string, error) {
	var resp map[string]string
	if err := c.post("/api/v1/executions/"+executionID+"/cancel", nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

type AuditEvent struct {
	ID             string `json:"id"`
	OrganisationID string `json:"organisation_id"`
	ActorID        string `json:"actor_id"`
	Action         string `json:"action"`
	TargetType     string `json:"target_type"`
	TargetID       string `json:"target_id"`
	Result         string `json:"result"`
	Metadata       string `json:"metadata"`
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
	b, _ := json.Marshal(body)
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
