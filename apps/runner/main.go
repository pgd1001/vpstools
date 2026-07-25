package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
	runnerToken := os.Getenv("VPS_RUNNER_TOKEN")
	knownHosts := os.Getenv("SSH_KNOWN_HOSTS")

	simulate := os.Getenv("SIMULATE") == "true" || os.Getenv("SIMULATE") == "1"

	if simulate {
		logger.Info("runner started in SIMULATE mode (no real SSH)")
	} else {
		logger.Info("runner started", "api_url", apiURL, "ssh_host", targetHost, "ssh_port", targetPort)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	hostname, _ := os.Hostname()
	runnerID := registerRunner(ctx, httpClient, apiURL, runnerName, hostname, runnerToken, logger)

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
			sendHeartbeat(ctx, httpClient, apiURL, runnerID, hostname, runnerToken, logger)
			lastHeartbeat = time.Now()
		}

		job, err := claimJob(ctx, httpClient, apiURL, runnerID, runnerToken)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		if job == nil {
			time.Sleep(pollInterval)
			continue
		}

		logger.Info("claimed job", "execution_id", job.ExecutionID, "target_id", job.TargetID)

		var result sshx.Result
		if simulate {
			result = simulateRun(job.Command)
		} else {
			host := job.Host
			port := job.Port
			user := job.User
			if host == "" {
				host = targetHost
			}
			if port == 0 {
				port = parsePort(targetPort)
			}
			if user == "" {
				user = sshUser
			}
			jobCtx, cancel := context.WithTimeout(ctx, time.Duration(job.Timeout)*time.Second)
			if job.Timeout <= 0 {
				cancel()
				jobCtx, cancel = context.WithTimeout(ctx, 5*time.Minute)
			}
			var executor *sshx.Executor
			if knownHosts == "" {
				result = sshx.Result{Error: "SSH_KNOWN_HOSTS is required when SIMULATE is disabled", ExitCode: -1}
			} else if secureExecutor, err := sshx.NewExecutorWithKnownHosts(fmt.Sprintf("%s:%d", host, port), user, sshPassword, knownHosts); err != nil {
				result = sshx.Result{Error: err.Error(), ExitCode: -1}
			} else {
				executor = secureExecutor
				result = executor.Run(jobCtx, job.Command)
			}
			cancel()
		}

		logger.Info("job completed", "execution_id", job.ExecutionID, "exit_code", result.ExitCode, "duration_ms", result.DurationMs)

		if err := submitResult(ctx, httpClient, apiURL, runnerID, job.TargetID, job.ExecutionID, runnerToken, result); err != nil {
			logger.Error("failed to submit result", "execution_id", job.ExecutionID, "error", err)
		}
	}
}

func registerRunner(ctx context.Context, client *http.Client, apiURL, name, hostname, token string, logger *slog.Logger) string {
	body := map[string]any{
		"name":        name,
		"hostname":    hostname,
		"platform":    runtime.GOOS,
		"version":     "0.2.0",
		"ip_address":  getOutboundIP(),
		"runner_type": "customer_managed",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runners", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VPS-Runner-Token", token)
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

func sendHeartbeat(ctx context.Context, client *http.Client, apiURL, runnerID, hostname, token string, logger *slog.Logger) {
	body := map[string]string{
		"runner_id": runnerID,
		"hostname":  hostname,
		"platform":  runtime.GOOS,
		"version":   "0.2.0",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runners/heartbeat", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VPS-Runner-Token", token)
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
	TargetID    string `json:"target_id"`
	ExecutionID string `json:"execution_id"`
	Command     string `json:"command"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Timeout     int    `json:"timeout"`
}

func claimJob(ctx context.Context, client *http.Client, apiURL, runnerID, token string) (*job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/api/v1/jobs/next?runner_id="+url.QueryEscape(runnerID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-VPS-Runner-Token", token)
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

func submitResult(ctx context.Context, client *http.Client, apiURL, runnerID, targetID, execID, token string, result sshx.Result) error {
	body := map[string]any{
		"runner_id":    runnerID,
		"target_id":    targetID,
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
	req.Header.Set("X-VPS-Runner-Token", token)
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

func parsePort(v string) int {
	var port int
	if _, err := fmt.Sscanf(v, "%d", &port); err != nil || port <= 0 {
		return 22
	}
	return port
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
