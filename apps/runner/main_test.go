package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgd1001/svrtools/packages/dispatch"
	"github.com/pgd1001/svrtools/packages/sshx"
)

type fakeDispatchDelivery struct {
	notification dispatch.Notification
	acked        bool
	nacked       bool
}

func (d *fakeDispatchDelivery) Notification() dispatch.Notification { return d.notification }
func (d *fakeDispatchDelivery) Ack(context.Context) error {
	d.acked = true
	return nil
}
func (d *fakeDispatchDelivery) Nak(context.Context, time.Duration) error {
	d.nacked = true
	return nil
}

type fakeDispatchConsumer struct {
	delivery dispatch.Delivery
	err      error
}

func (c *fakeDispatchConsumer) Next(context.Context) (dispatch.Delivery, error) {
	return c.delivery, c.err
}
func (c *fakeDispatchConsumer) Close() error { return nil }

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
	got, boundToken := registerRunner(context.Background(), server.Client(), server.URL, "test", "host", "token", logger)
	if got != "" {
		t.Fatalf("expected rejected registration to return no runner id, got %q", got)
	}
	if boundToken != "" {
		t.Fatalf("expected rejected registration to return no credential, got %q", boundToken)
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
		_, _ = w.Write([]byte(`{"runner_id":"run_test","status":"active","runner_token":"bound_test_token"}`))
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	got, boundToken := registerRunner(context.Background(), server.Client(), server.URL, "test", "host", "token", logger)
	if got != "run_test" {
		t.Fatalf("expected runner id, got %q", got)
	}
	// The runner must adopt the identity-bound credential registration issues,
	// because the job endpoints reject the bootstrap credential.
	if boundToken != "bound_test_token" {
		t.Fatalf("expected the bound credential, got %q", boundToken)
	}
}

func TestClaimNotificationClaimsTargetAndAcknowledgesBeforeExecution(t *testing.T) {
	delivery := &fakeDispatchDelivery{notification: dispatch.Notification{
		Version: dispatch.NotificationVersion, Kind: dispatch.NotificationKind,
		TargetID: "target-1", ExecutionID: "execution-1",
	}}
	consumer := &fakeDispatchConsumer{delivery: delivery}
	claimed, handled, err := claimNotification(context.Background(), consumer, func(targetID string) (*job, error) {
		if targetID != "target-1" {
			t.Fatalf("claim target = %q", targetID)
		}
		return &job{TargetID: targetID, ExecutionID: "execution-1", LeaseID: "lease-1"}, nil
	})
	if err != nil || !handled || claimed == nil {
		t.Fatalf("unexpected result: job=%#v handled=%v err=%v", claimed, handled, err)
	}
	if !delivery.acked || delivery.nacked {
		t.Fatalf("notification acknowledgement state: acked=%v nacked=%v", delivery.acked, delivery.nacked)
	}
}

func TestClaimNotificationNacksWhenAPIClaimFails(t *testing.T) {
	delivery := &fakeDispatchDelivery{notification: dispatch.Notification{
		Version: dispatch.NotificationVersion, Kind: dispatch.NotificationKind,
		TargetID: "target-1", ExecutionID: "execution-1",
	}}
	consumer := &fakeDispatchConsumer{delivery: delivery}
	claimed, handled, err := claimNotification(context.Background(), consumer, func(string) (*job, error) {
		return nil, errors.New("API unavailable")
	})
	if err == nil || claimed != nil || !handled {
		t.Fatalf("expected failed claim, got job=%#v handled=%v err=%v", claimed, handled, err)
	}
	if delivery.acked || !delivery.nacked {
		t.Fatalf("notification acknowledgement state: acked=%v nacked=%v", delivery.acked, delivery.nacked)
	}
}

func TestClaimNotificationFallsBackWhenNoNotificationIsAvailable(t *testing.T) {
	consumer := &fakeDispatchConsumer{err: dispatch.ErrNoMessage}
	claimed, handled, err := claimNotification(context.Background(), consumer, func(string) (*job, error) {
		t.Fatal("claim function must not run without a notification")
		return nil, nil
	})
	if err != nil || handled || claimed != nil {
		t.Fatalf("unexpected no-message result: job=%#v handled=%v err=%v", claimed, handled, err)
	}
}
