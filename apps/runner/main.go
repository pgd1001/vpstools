package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/pgd1001/svrtools/packages/redact"
	"github.com/pgd1001/svrtools/packages/sshx"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	runnerName := envOrDefault("RUNNER_NAME", "default-runner")

	targetHost := envOrDefault("SSH_TARGET_HOST", "localhost")
	targetPort := envOrDefault("SSH_TARGET_PORT", "2222")
	sshUser := envOrDefault("SSH_USER", "svrtools")
	sshPassword := envOrDefault("SSH_PASSWORD", "svrtools")

	simulate := os.Getenv("SIMULATE") == "true" || os.Getenv("SIMULATE") == "1"

	if simulate {
		logger.Info("runner started in SIMULATE mode (no real SSH)")
	} else {
		logger.Info("runner started", "api_url", apiURL, "ssh_host", targetHost, "ssh_port", targetPort)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	var executor *sshx.Executor
	if !simulate {
		executor = sshx.NewExecutor(targetHost+":"+targetPort, sshUser, sshPassword)
	}

	hostname, _ := os.Hostname()
	runnerID := registerRunner(ctx, httpClient, apiURL, runnerName, hostname, logger)

	lastHeartbeat := time.Now()
	pollInterval := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			logger.Info("runner shutting down")
			return
		default:
		}

		if time.Since(lastHeartbeat) > 30*time.Second && runnerID != "" {
			sendHeartbeat(ctx, httpClient, apiURL, runnerID, hostname, logger)
			lastHeartbeat = time.Now()
		}

		job, err := claimJob(ctx, httpClient, apiURL)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		if job == nil {
			time.Sleep(pollInterval)
			continue
		}

		logger.Info("claimed job", "execution_id", job.ExecutionID, "command", job.Command)

		var result sshx.Result
		if simulate {
			result = simulateRun(job.Command)
		} else {
			result = executor.Run(ctx, job.Command)
		}

		logger.Info("job completed", "execution_id", job.ExecutionID, "exit_code", result.ExitCode, "duration_ms", result.DurationMs)

		if err := submitResult(ctx, httpClient, apiURL, job.ExecutionID, result); err != nil {
			logger.Error("failed to submit result", "execution_id", job.ExecutionID, "error", err)
		}
	}
}

func registerRunner(ctx context.Context, client *http.Client, apiURL, name, hostname string, logger *slog.Logger) string {
	body := map[string]any{
		"name":      name,
		"hostname":  hostname,
		"platform":  runtime.GOOS,
		"version":   "0.2.0",
		"ip_address": getOutboundIP(),
		"runner_type": "customer_managed",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runners", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("runner registration failed, continuing unregistered", "error", err)
		return ""
	}
	defer resp.Body.Close()
	var regResp struct {
		RunnerID string `json:"runner_id"`
		Status   string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&regResp)
	logger.Info("runner registered with API", "runner_id", regResp.RunnerID)
	return regResp.RunnerID
}

func sendHeartbeat(ctx context.Context, client *http.Client, apiURL, runnerID, hostname string, logger *slog.Logger) {
	body := map[string]string{
		"runner_id": runnerID,
		"hostname":  hostname,
		"platform":  runtime.GOOS,
		"version":   "0.2.0",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runners/heartbeat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("heartbeat failed", "error", err)
		return
	}
	defer resp.Body.Close()
	logger.Info("heartbeat sent", "runner_id", runnerID)
}

func getOutboundIP() string {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "127.0.0.1"
	}
	defer resp.Body.Close()
	var ip string
	json.NewDecoder(resp.Body).Decode(&ip)
	return ip
}

func simulateRun(command string) sshx.Result {
	start := time.Now()
	time.Sleep(100 * time.Millisecond)
	return sshx.Result{
		Stdout:     fmt.Sprintf("[simulated] %s\n", command),
		ExitCode:   0,
		DurationMs: time.Since(start).Milliseconds(),
	}
}

type job struct {
	ExecutionID string `json:"execution_id"`
	Command     string `json:"command"`
	Target      string `json:"target"`
}

func claimJob(ctx context.Context, client *http.Client, apiURL string) (*job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/api/v1/jobs/next?organisation_id=org_demo", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var j job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	if j.ExecutionID == "" {
		return nil, nil
	}
	return &j, nil
}

func submitResult(ctx context.Context, client *http.Client, apiURL, execID string, result sshx.Result) error {
	body := map[string]any{
		"execution_id": execID,
		"exit_code":    result.ExitCode,
		"stdout":       redact.Stdout(result.Stdout),
		"stderr":       redact.Stdout(result.Stderr),
		"error":        result.Error,
		"duration_ms":  result.DurationMs,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/jobs/result", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

