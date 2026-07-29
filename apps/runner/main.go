package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pgd1001/svrtools/packages/config"
	"github.com/pgd1001/svrtools/packages/dispatch"
	"github.com/pgd1001/svrtools/packages/redact"
	"github.com/pgd1001/svrtools/packages/sshx"
)

var version = "0.1.0-beta.1"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	health := &runnerHealth{}
	stopHealth := startHealthServer(ctx, os.Getenv("RUNNER_HEALTH_ADDR"), health, logger)
	defer stopHealth()

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	runnerName := envOrDefault("RUNNER_NAME", "default-runner")
	backendConfig := config.Load()
	if err := backendConfig.Validate(); err != nil {
		logger.Error("runner backend configuration invalid", "error", err)
		os.Exit(1)
	}

	targetHost := envOrDefault("SSH_TARGET_HOST", "localhost")
	targetPort := envOrDefault("SSH_TARGET_PORT", "2222")
	sshUser := envOrDefault("SSH_USER", "svrtools")
	sshPassword := os.Getenv("SSH_PASSWORD")
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
	if runnerID == "" {
		logger.Error("runner cannot start without successful registration")
		os.Exit(1)
	}
	health.registered.Store(true)

	var dispatchConsumer dispatch.Consumer
	if backendConfig.JobDispatch == "jetstream" {
		natsURL, stream, subject, durable, maxDeliver, ackWait, duplicateWindow := backendConfig.DispatchConfig()
		consumer, err := dispatch.NewJetStreamConsumer(ctx, dispatch.Config{
			URL: natsURL, Stream: stream, Subject: subject, Durable: durable,
			MaxDeliver: maxDeliver, AckWait: ackWait, DuplicateWindow: duplicateWindow,
		})
		if err != nil {
			logger.Error("JetStream dispatch initialisation failed", "error", err)
			os.Exit(1)
		}
		dispatchConsumer = consumer
		defer dispatchConsumer.Close()
		logger.Info("runner using JetStream notification bridge", "stream", stream, "subject", subject, "durable", durable, "max_deliver", maxDeliver)
	}

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

		var claimedJob *job
		var err error
		var notificationHandled bool
		if dispatchConsumer != nil {
			claimedJob, notificationHandled, err = claimNotification(ctx, dispatchConsumer, func(targetID string) (*job, error) {
				return claimJobForTarget(ctx, httpClient, apiURL, runnerID, runnerToken, targetID)
			})
			if err != nil {
				logger.Warn("JetStream notification processing failed", "error", err)
			}
		}
		if dispatchConsumer == nil || !notificationHandled {
			claimedJob, err = claimJob(ctx, httpClient, apiURL, runnerID, runnerToken)
		}
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		if claimedJob == nil {
			time.Sleep(pollInterval)
			continue
		}

		job := claimedJob
		logger.Info("claimed job", "execution_id", job.ExecutionID, "target_id", job.TargetID)
		health.claimed.Add(1)
		leaseCtx, stopLeaseRenewal := context.WithCancel(ctx)
		leaseRenewed := make(chan struct{})
		go renewLeaseLoop(leaseCtx, httpClient, apiURL, runnerID, job, runnerToken, logger, leaseRenewed)

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
		stopLeaseRenewal()
		<-leaseRenewed

		logger.Info("job completed", "execution_id", job.ExecutionID, "exit_code", result.ExitCode, "duration_ms", result.DurationMs)
		health.completed.Add(1)
		health.lastCompletionUnix.Store(time.Now().Unix())

		if err := submitResultWithRetry(ctx, httpClient, apiURL, runnerID, job.TargetID, job.ExecutionID, job.LeaseID, runnerToken, result); err != nil {
			logger.Error("failed to submit result", "execution_id", job.ExecutionID, "error", err)
		}
	}
}

type runnerHealth struct {
	registered         atomic.Bool
	claimed            atomic.Uint64
	completed          atomic.Uint64
	lastCompletionUnix atomic.Int64
}

func startHealthServer(ctx context.Context, address string, state *runnerHealth, logger *slog.Logger) func() {
	if address == "" {
		return func() {}
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Error("runner health endpoint failed to start", "address", address, "error", err)
		return func() {}
	}
	mux := runnerHealthMux(state)
	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("runner health endpoint stopped", "error", err)
		}
	}()
	logger.Info("runner health endpoint started", "address", address)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
}

func runnerHealthMux(state *runnerHealth) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if !state.registered.Load() {
			writeRunnerHealthJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "starting"})
			return
		}
		writeRunnerHealthJSON(w, http.StatusOK, map[string]any{"status": "healthy", "jobs_claimed": state.claimed.Load(), "jobs_completed": state.completed.Load(), "last_completion_unix": state.lastCompletionUnix.Load()})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "# HELP svrtools_runner_registered Runner has registered with the control plane.\n# TYPE svrtools_runner_registered gauge\nsvrtools_runner_registered %d\n", boolMetric(state.registered.Load()))
		fmt.Fprintf(w, "# HELP svrtools_runner_jobs_claimed_total Jobs claimed by this runner.\n# TYPE svrtools_runner_jobs_claimed_total counter\nsvrtools_runner_jobs_claimed_total %d\n", state.claimed.Load())
		fmt.Fprintf(w, "# HELP svrtools_runner_jobs_completed_total Jobs completed by this runner.\n# TYPE svrtools_runner_jobs_completed_total counter\nsvrtools_runner_jobs_completed_total %d\n", state.completed.Load())
	})
	return mux
}

func writeRunnerHealthJSON(w http.ResponseWriter, status int, value map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func boolMetric(value bool) int {
	if value {
		return 1
	}
	return 0
}

func renewLeaseLoop(ctx context.Context, client *http.Client, apiURL, runnerID string, job *job, token string, logger *slog.Logger, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := renewLease(ctx, client, apiURL, runnerID, job, token); err != nil {
				logger.Warn("lease renewal failed", "execution_id", job.ExecutionID, "target_id", job.TargetID, "lease_id", job.LeaseID, "error", err)
			} else {
				logger.Debug("lease renewed", "execution_id", job.ExecutionID, "target_id", job.TargetID, "lease_id", job.LeaseID)
			}
		}
	}
}

func renewLease(ctx context.Context, client *http.Client, apiURL, runnerID string, job *job, token string) error {
	body, err := json.Marshal(map[string]string{
		"execution_id": job.ExecutionID,
		"target_id":    job.TargetID,
		"runner_id":    runnerID,
		"lease_id":     job.LeaseID,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/jobs/renew", bytes.NewReader(body))
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
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func registerRunner(ctx context.Context, client *http.Client, apiURL, name, hostname, token string, logger *slog.Logger) string {
	body := map[string]any{
		"name":        name,
		"hostname":    hostname,
		"platform":    runtime.GOOS,
		"version":     version,
		"ip_address":  getOutboundIP(),
		"runner_type": "customer_managed",
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/api/v1/runners", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-VPS-Runner-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("runner registration failed", "error", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("runner registration rejected", "status", resp.StatusCode)
		return ""
	}
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
		"version":   version,
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("heartbeat rejected", "runner_id", runnerID, "status", resp.StatusCode)
		return
	}
	logger.Info("heartbeat sent", "runner_id", runnerID)
}

func getOutboundIP() string {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
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
	LeaseID     string `json:"lease_id"`
}

type resultSubmissionError struct {
	status int
}

func (e *resultSubmissionError) Error() string {
	return fmt.Sprintf("unexpected status: %d", e.status)
}

func claimJob(ctx context.Context, client *http.Client, apiURL, runnerID, token string) (*job, error) {
	return claimJobForTarget(ctx, client, apiURL, runnerID, token, "")
}

func claimJobForTarget(ctx context.Context, client *http.Client, apiURL, runnerID, token, targetID string) (*job, error) {
	claimURL := apiURL + "/api/v1/jobs/next?runner_id=" + url.QueryEscape(runnerID)
	if targetID != "" {
		claimURL += "&target_id=" + url.QueryEscape(targetID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claimURL, nil)
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

// claimNotification consumes a notification, asks the API to claim exactly
// the referenced target, and acknowledges the notification before execution.
// A duplicate notification therefore results in a harmless no_jobs response
// once the API lease has already been claimed or completed.
func claimNotification(ctx context.Context, consumer dispatch.Consumer, claim func(string) (*job, error)) (*job, bool, error) {
	delivery, err := consumer.Next(ctx)
	if errors.Is(err, dispatch.ErrNoMessage) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	notification := delivery.Notification()
	claimed, claimErr := claim(notification.TargetID)
	if claimErr != nil {
		nakErr := delivery.Nak(ctx, time.Second)
		if nakErr != nil {
			return nil, true, fmt.Errorf("claim target %s: %v, then nack notification: %w", notification.TargetID, claimErr, nakErr)
		}
		return nil, true, claimErr
	}
	ackErr := delivery.Ack(ctx)
	if ackErr != nil {
		if claimed != nil {
			return claimed, true, fmt.Errorf("ack notification for claimed target %s: %w", notification.TargetID, ackErr)
		}
		return nil, true, ackErr
	}
	return claimed, true, nil
}

func submitResult(ctx context.Context, client *http.Client, apiURL, runnerID, targetID, execID, leaseID, token string, result sshx.Result) error {
	body := map[string]any{
		"runner_id":    runnerID,
		"target_id":    targetID,
		"execution_id": execID,
		"lease_id":     leaseID,
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
		return &resultSubmissionError{status: resp.StatusCode}
	}
	return nil
}

func submitResultWithRetry(ctx context.Context, client *http.Client, apiURL, runnerID, targetID, execID, leaseID, token string, result sshx.Result) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := submitResult(ctx, client, apiURL, runnerID, targetID, execID, leaseID, token, result); err == nil {
			return nil
		} else {
			lastErr = err
			if statusErr, ok := err.(*resultSubmissionError); ok && statusErr.status != http.StatusRequestTimeout && statusErr.status != http.StatusTooManyRequests && statusErr.status < http.StatusInternalServerError {
				return err
			}
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
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
