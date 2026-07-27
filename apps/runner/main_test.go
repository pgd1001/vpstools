package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pgd1001/svrtools/packages/sshx"
)

func TestRunnerHealthMuxReportsRegistrationAndCounters(t *testing.T) {
	state := &runnerHealth{}
	mux := runnerHealthMux(state)
	starting := httptest.NewRecorder()
	mux.ServeHTTP(starting, httptest.NewRequest(http.MethodGet, "/health", nil))
	if starting.Code != http.StatusServiceUnavailable {
		t.Fatalf("unregistered runner health should be unavailable, got %d", starting.Code)
	}

	state.registered.Store(true)
	state.claimed.Store(2)
	state.completed.Store(1)
	health := httptest.NewRecorder()
	mux.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("registered runner health should be healthy, got %d", health.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(health.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if payload["status"] != "healthy" || payload["jobs_claimed"] != float64(2) || payload["jobs_completed"] != float64(1) {
		t.Fatalf("unexpected health payload: %#v", payload)
	}

	metrics := httptest.NewRecorder()
	mux.ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, expected := range []string{"svrtools_runner_registered 1", "svrtools_runner_jobs_claimed_total 2", "svrtools_runner_jobs_completed_total 1"} {
		if !strings.Contains(metrics.Body.String(), expected) {
			t.Fatalf("runner metrics missing %q: %s", expected, metrics.Body.String())
		}
	}
}

func TestRegisterRunnerRejectsNonSuccessResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	if got := registerRunner(context.Background(), server.Client(), server.URL, "test", "host", "token", logger); got != "" {
		t.Fatalf("expected rejected registration to return no runner id, got %q", got)
	}
}

func TestSubmitResultWithRetryReplaysIdenticalPayloadAfterTransientFailure(t *testing.T) {
	var requests atomic.Int32
	var firstBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if requests.Add(1) == 1 {
			firstBody = body
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if !bytes.Equal(firstBody, body) {
			t.Errorf("retry payload changed: first=%s retry=%s", firstBody, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	err := submitResultWithRetry(context.Background(), server.Client(), server.URL, "runner", "target", "execution", "lease", "token", sshx.Result{ExitCode: 0, Stdout: "ok"})
	if err != nil {
		t.Fatalf("expected transient submission to recover, got %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two submission attempts, got %d", requests.Load())
	}
}

func TestSubmitResultWithRetryDoesNotRetryPermanentFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	err := submitResultWithRetry(context.Background(), server.Client(), server.URL, "runner", "target", "execution", "lease", "token", sshx.Result{ExitCode: 0})
	if err == nil {
		t.Fatal("expected permanent submission failure")
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one submission attempt, got %d", requests.Load())
	}
}

func TestRegisterRunnerReturnsIdForSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-VPS-Runner-Token") != "token" {
			t.Fatalf("runner token header missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"runner_id":"run_test","status":"active"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	if got := registerRunner(context.Background(), server.Client(), server.URL, "test", "host", "token", logger); got != "run_test" {
		t.Fatalf("expected runner id, got %q", got)
	}
}
