package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Client struct {
	baseURL     string
	userID      string
	apiToken    string
	runnerToken string
	http        *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL:     baseURL,
		userID:      "",
		apiToken:    os.Getenv("VPS_API_TOKEN"),
		runnerToken: os.Getenv("VPS_RUNNER_TOKEN"),
		http:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) SetUser(userID string) {
	c.userID = userID
}

// SetToken configures a bearer token for production API access. When set, the
// client does not send the development X-VPS-User header.
func (c *Client) SetToken(token string) {
	c.apiToken = token
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
		path += fmt.Sprintf("%senvironment=%s", sep, url.QueryEscape(environment))
		sep = "&"
	}
	if tagKey != "" {
		path += fmt.Sprintf("%stag_key=%s", sep, url.QueryEscape(tagKey))
		sep = "&"
	}
	if tagValue != "" {
		path += fmt.Sprintf("%stag_value=%s", sep, url.QueryEscape(tagValue))
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
	if err := c.get("/api/v1/servers/"+url.PathEscape(serverID), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type CheckServerResponse struct {
	Server map[string]any `json:"server"`
}

func (c *Client) CheckServer(serverID string) (*CheckServerResponse, error) {
	var resp CheckServerResponse
	if err := c.post("/api/v1/servers/"+url.PathEscape(serverID)+"/check", nil, &resp); err != nil {
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
	return c.CreateRegistrationTokenForRunner("")
}

func (c *Client) CreateRegistrationTokenForRunner(runnerID string) (*CreateRegistrationTokenResponse, error) {
	var resp CreateRegistrationTokenResponse
	body := any(nil)
	if runnerID != "" {
		body = map[string]string{"runner_id": runnerID}
	}
	if err := c.post("/api/v1/runners/registration-token", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) RotateRunnerToken(runnerID string) (*CreateRegistrationTokenResponse, error) {
	var resp CreateRegistrationTokenResponse
	if err := c.post("/api/v1/runners/"+url.PathEscape(runnerID)+"/rotate-token", nil, &resp); err != nil {
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
	return c.CreateExecutionWithIdempotencyKey(target, command, reason, "")
}

// CreateExecutionWithIdempotencyKey makes a retried submission safe. The API
// persists the key with the original request payload and replays the original
// response when the same key and payload are sent again.
func (c *Client) CreateExecutionWithIdempotencyKey(target, command, reason, idempotencyKey string) (*CreateExecutionResponse, error) {
	body := CreateExecutionRequest{
		Target:  target,
		Command: command,
		Reason:  reason,
	}
	var resp CreateExecutionResponse
	if err := c.postWithHeaders("/api/v1/executions", body, &resp, map[string]string{"Idempotency-Key": idempotencyKey}); err != nil {
		return nil, err
	}
	return &resp, nil
}

type ExecutionDetail struct {
	ID             string `json:"id"`
	ActorUserID    string `json:"actor_user_id"`
	ActorRole      string `json:"actor_role_at_time"`
	ExecutionType  string `json:"execution_type"`
	Status         string `json:"status"`
	RiskLevel      string `json:"risk_level"`
	Environment    string `json:"environment"`
	Reason         string `json:"reason"`
	CommandPreview string `json:"command_preview"`
	CommandHash    string `json:"command_hash"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	RequestedAt    string `json:"requested_at"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	ErrorSummary   string `json:"error_summary"`
	DelegatedBy    string `json:"delegated_by_user_id"`
	ApprovalID     string `json:"approval_id"`
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
	if err := c.get("/api/v1/executions/"+url.PathEscape(executionID), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type ExecutionListItem struct {
	ID             string `json:"id"`
	ActorUserID    string `json:"actor_user_id"`
	ActorRole      string `json:"actor_role_at_time"`
	ExecutionType  string `json:"execution_type"`
	Status         string `json:"status"`
	CommandPreview string `json:"command_preview"`
	TargetCount    int    `json:"target_count"`
	SucceededCount int    `json:"succeeded_count"`
	FailedCount    int    `json:"failed_count"`
	RequestedAt    string `json:"requested_at"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	DelegatedBy    string `json:"delegated_by_user_id"`
	ApprovalID     string `json:"approval_id"`
}

type ListExecutionsResponse struct {
	Executions []ExecutionListItem `json:"executions"`
}

func (c *Client) ListExecutions(status, limit string) (*ListExecutionsResponse, error) {
	path := "/api/v1/executions"
	sep := "?"
	if status != "" {
		path += fmt.Sprintf("%sstatus=%s", sep, url.QueryEscape(status))
		sep = "&"
	}
	if limit != "" {
		path += fmt.Sprintf("%slimit=%s", sep, url.QueryEscape(limit))
	}
	var resp ListExecutionsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListMyExecutions(status, limit string) (*ListExecutionsResponse, error) {
	path := "/api/v1/executions?mine=true"
	if status != "" {
		path += fmt.Sprintf("&status=%s", url.QueryEscape(status))
	}
	if limit != "" {
		path += fmt.Sprintf("&limit=%s", url.QueryEscape(limit))
	}
	var resp ListExecutionsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListDelegatedExecutions(status, limit string) (*ListExecutionsResponse, error) {
	path := "/api/v1/executions?delegated=true"
	if status != "" {
		path += fmt.Sprintf("&status=%s", url.QueryEscape(status))
	}
	if limit != "" {
		path += fmt.Sprintf("&limit=%s", url.QueryEscape(limit))
	}
	var resp ListExecutionsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CancelExecution(executionID string) (map[string]string, error) {
	var resp map[string]string
	if err := c.post("/api/v1/executions/"+url.PathEscape(executionID)+"/cancel", nil, &resp); err != nil {
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

type VerifyAuditResponse struct {
	Valid         bool   `json:"valid"`
	CheckedEvents int    `json:"checked_events"`
	Error         string `json:"error,omitempty"`
}

func (c *Client) ListAudit(limit string) (*ListAuditResponse, error) {
	var resp ListAuditResponse
	if err := c.get(fmt.Sprintf("/api/v1/audit?limit=%s", url.QueryEscape(limit)), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) VerifyAudit() (*VerifyAuditResponse, error) {
	var resp VerifyAuditResponse
	if err := c.get("/api/v1/audit/verify", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type Schedule struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	RunbookName     string `json:"runbook_name"`
	Target          string `json:"target"`
	Reason          string `json:"reason"`
	Params          string `json:"params"`
	IntervalSeconds int    `json:"interval_seconds"`
	NextRunAt       string `json:"next_run_at"`
	Enabled         bool   `json:"enabled"`
	LastRunAt       string `json:"last_run_at"`
	LastError       string `json:"last_error"`
}

type ListSchedulesResponse struct {
	Schedules []Schedule `json:"schedules"`
}

type CreateScheduleRequest struct {
	Name            string            `json:"name"`
	RunbookName     string            `json:"runbook_name"`
	Target          string            `json:"target"`
	Reason          string            `json:"reason"`
	Params          map[string]string `json:"params"`
	IntervalSeconds int               `json:"interval_seconds"`
	NextRunAt       string            `json:"next_run_at,omitempty"`
}

func (c *Client) ListSchedules() (*ListSchedulesResponse, error) {
	var resp ListSchedulesResponse
	if err := c.get("/api/v1/schedules", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) CreateSchedule(req CreateScheduleRequest) (map[string]string, error) {
	var resp map[string]string
	if err := c.post("/api/v1/schedules", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) DisableSchedule(scheduleID string) (map[string]string, error) {
	var resp map[string]string
	if err := c.delete("/api/v1/schedules/"+url.PathEscape(scheduleID), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

type AutomationStatus struct {
	Paused   bool   `json:"paused"`
	PausedAt string `json:"paused_at,omitempty"`
	PausedBy string `json:"paused_by,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func (c *Client) AutomationStatus() (*AutomationStatus, error) {
	var resp AutomationStatus
	if err := c.get("/api/v1/automation/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PauseAutomation(reason string) (*AutomationStatus, error) {
	var resp AutomationStatus
	if err := c.post("/api/v1/automation/pause", map[string]string{"reason": reason}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ResumeAutomation() (*AutomationStatus, error) {
	var resp AutomationStatus
	if err := c.post("/api/v1/automation/resume", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	c.setAuth(req)
	if c.runnerToken != "" {
		req.Header.Set("X-VPS-Runner-Token", c.runnerToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return c.parseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(path string, body, out any) error {
	return c.postWithHeaders(path, body, out, nil)
}

func (c *Client) postWithHeaders(path string, body, out any, headers map[string]string) error {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	c.setAuth(req)
	if c.runnerToken != "" {
		req.Header.Set("X-VPS-Runner-Token", c.runnerToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) delete(path string, out any) error {
	req, _ := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	c.setAuth(req)
	if c.runnerToken != "" {
		req.Header.Set("X-VPS-Runner-Token", c.runnerToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseError(resp)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type CreateAPITokenResponse struct {
	Token          string `json:"token"`
	TokenID        string `json:"token_id"`
	UserID         string `json:"user_id"`
	OrganisationID string `json:"organisation_id"`
	ExpiresAt      string `json:"expires_at"`
}

func (c *Client) CreateAPIToken(name, userID string, expiresIn int) (*CreateAPITokenResponse, error) {
	var resp CreateAPITokenResponse
	body := map[string]any{"name": name, "user_id": userID, "expires_in": expiresIn}
	if err := c.post("/api/v1/auth/tokens", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) RevokeAPIToken(tokenID string) error {
	return c.delete("/api/v1/auth/tokens/"+url.PathEscape(tokenID), &map[string]any{})
}

func (c *Client) setAuth(req *http.Request) {
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		return
	}
	if c.userID != "" {
		req.Header.Set("X-VPS-User", c.userID)
	}
}

func (c *Client) parseError(resp *http.Response) error {
	var apiErr struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
		Next   string `json:"next"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && apiErr.Error != "" {
		msg := apiErr.Error
		if apiErr.Next != "" {
			msg += "\n" + apiErr.Next
		}
		return fmt.Errorf("%s", msg)
	}
	return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

type RunbookItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Risk         string `json:"risk_level"`
	Command      string `json:"command_preview"`
	CreatedAt    string `json:"created_at"`
	Permitted    bool   `json:"permitted"`
	AllowedRoles string `json:"allowed_roles"`
}

type ListRunbooksResponse struct {
	Runbooks []RunbookItem `json:"runbooks"`
}

func (c *Client) ListRunbooks() (*ListRunbooksResponse, error) {
	return c.SearchRunbooks("")
}

func (c *Client) SearchRunbooks(query string) (*ListRunbooksResponse, error) {
	path := "/api/v1/runbooks"
	if query != "" {
		path += "?search=" + url.QueryEscape(query)
	}
	var resp ListRunbooksResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type RunbookDetail struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Version      int    `json:"version"`
	Risk         string `json:"risk_level"`
	Command      string `json:"command"`
	Definition   string `json:"definition_json"`
	AllowedRoles string `json:"allowed_roles"`
	CreatedAt    string `json:"created_at"`
}

type GetRunbookResponse struct {
	Runbook RunbookDetail `json:"runbook"`
}

func (c *Client) GetRunbook(name string) (*GetRunbookResponse, error) {
	var resp GetRunbookResponse
	if err := c.get("/api/v1/runbooks/"+url.PathEscape(name), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type CreateRunbookRequest struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Risk        string `json:"risk"`
	Command     string `json:"command"`
	Timeout     int    `json:"timeout"`
	Environment string `json:"environment"`
	YAML        string `json:"yaml"`
}

type CreateRunbookResponse struct {
	RunbookID string `json:"runbook_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

func (c *Client) CreateRunbook(req CreateRunbookRequest) (*CreateRunbookResponse, error) {
	var resp CreateRunbookResponse
	if err := c.post("/api/v1/runbooks", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) PublishRunbook(name string) (map[string]string, error) {
	var resp map[string]string
	if err := c.post("/api/v1/runbooks/"+url.PathEscape(name)+"/publish", nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) RunRunbook(name, target, reason string, params map[string]string) (map[string]any, error) {
	return c.RunRunbookWithIdempotencyKey(name, target, reason, params, "")
}

// RunRunbookWithIdempotencyKey makes a retried runbook submission safe. This
// also covers approval requests, not only direct execution creation.
func (c *Client) RunRunbookWithIdempotencyKey(name, target, reason string, params map[string]string, idempotencyKey string) (map[string]any, error) {
	return c.runRunbook(name, target, reason, params, false, idempotencyKey)
}

func (c *Client) PreflightRunbook(name, target, reason string, params map[string]string) (map[string]any, error) {
	return c.runRunbook(name, target, reason, params, true, "")
}

func (c *Client) runRunbook(name, target, reason string, params map[string]string, dryRun bool, idempotencyKey string) (map[string]any, error) {
	body := map[string]any{
		"target":  target,
		"reason":  reason,
		"params":  params,
		"dry_run": dryRun,
	}
	var resp map[string]any
	if err := c.postWithHeaders("/api/v1/runbooks/"+url.PathEscape(name)+"/run", body, &resp, map[string]string{"Idempotency-Key": idempotencyKey}); err != nil {
		return nil, err
	}
	return resp, nil
}

type ApprovalItem struct {
	ID            string `json:"id"`
	RequesterName string `json:"requester_name"`
	ActionType    string `json:"action_type"`
	Status        string `json:"status"`
	RiskLevel     string `json:"risk_level"`
	Reason        string `json:"reason"`
	TargetType    string `json:"target_type"`
	TargetID      string `json:"target_id"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
}

type ListApprovalsResponse struct {
	Approvals []ApprovalItem `json:"approvals"`
}

type ApprovalDetail struct {
	ID             string         `json:"id"`
	RequesterID    string         `json:"requester_id"`
	RequesterName  string         `json:"requester_name"`
	ActionType     string         `json:"action_type"`
	Status         string         `json:"status"`
	RiskLevel      string         `json:"risk_level"`
	Reason         string         `json:"reason"`
	TargetType     string         `json:"target_type"`
	TargetID       string         `json:"target_id"`
	TargetSnapshot string         `json:"target_snapshot"`
	RequestPayload map[string]any `json:"request_payload"`
	ExpiresAt      string         `json:"expires_at"`
	CreatedAt      string         `json:"created_at"`
	DecidedAt      string         `json:"decided_at"`
	DecisionNote   string         `json:"decision_note"`
}

func (c *Client) GetApproval(approvalID string) (*ApprovalDetail, error) {
	var resp struct {
		Approval ApprovalDetail `json:"approval"`
	}
	if err := c.get("/api/v1/approvals/"+url.PathEscape(approvalID), &resp); err != nil {
		return nil, err
	}
	return &resp.Approval, nil
}

func (c *Client) ListApprovals(status string) (*ListApprovalsResponse, error) {
	path := "/api/v1/approvals"
	if status != "" {
		path += "?status=" + url.QueryEscape(status)
	}
	var resp ListApprovalsResponse
	if err := c.get(path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ApproveApproval(approvalID string, note ...string) (map[string]string, error) {
	var resp map[string]string
	body := approvalNoteBody(note)
	if err := c.post("/api/v1/approvals/"+url.PathEscape(approvalID)+"/approve", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) DenyApproval(approvalID string, note ...string) (map[string]string, error) {
	var resp map[string]string
	body := approvalNoteBody(note)
	if err := c.post("/api/v1/approvals/"+url.PathEscape(approvalID)+"/deny", body, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func approvalNoteBody(note []string) any {
	if len(note) == 0 || note[0] == "" {
		return nil
	}
	return map[string]string{"note": note[0]}
}
