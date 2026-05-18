package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pgd1001/svrtools/packages/sshx"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	targetHost := envOrDefault("SSH_TARGET_HOST", "localhost")
	targetPort := envOrDefault("SSH_TARGET_PORT", "2222")
	sshUser := envOrDefault("SSH_USER", "svrtools")
	sshPassword := envOrDefault("SSH_PASSWORD", "svrtools")

	simulate := os.Getenv("SIMULATE") == "true" || os.Getenv("SIMULATE") == "1"

	if simulate {
		log.Println("runner started in SIMULATE mode (no real SSH)")
	} else {
		log.Printf("runner started: API=%s, SSH=%s:%s", apiURL, targetHost, targetPort)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var executor *sshx.Executor
	if !simulate {
		executor = sshx.NewExecutor(targetHost+":"+targetPort, sshUser, sshPassword)
	}

	pollInterval := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			log.Println("runner shutting down")
			return
		default:
		}

		job, err := claimJob(ctx, client, apiURL)
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		if job == nil {
			time.Sleep(pollInterval)
			continue
		}

		log.Printf("claimed job %s: %s", job.ExecutionID, job.Command)

		var result sshx.Result
		if simulate {
			result = simulateRun(job.Command)
		} else {
			result = executor.Run(ctx, job.Command)
		}

		log.Printf("job %s completed: exit=%d, duration=%dms", job.ExecutionID, result.ExitCode, result.DurationMs)

		if err := submitResult(ctx, client, apiURL, job.ExecutionID, result); err != nil {
			log.Printf("failed to submit result for %s: %v", job.ExecutionID, err)
		}
	}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"/api/v1/jobs/next", nil)
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
		"stdout":       result.Stdout,
		"stderr":       result.Stderr,
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
