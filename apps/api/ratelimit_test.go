package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBoundedRateLimiterEnforcesWindowAndCapsEntries(t *testing.T) {
	now := time.Unix(100, 0)
	limiter := newBoundedRateLimiter(2, 2, time.Minute)
	limiter.now = func() time.Time { return now }

	if !limiter.allow("a") || !limiter.allow("a") || limiter.allow("a") {
		t.Fatal("expected the third request in a window to be rejected")
	}
	if !limiter.allow("b") || !limiter.allow("c") {
		t.Fatal("expected new keys to be admitted")
	}
	if len(limiter.entries) != 2 {
		t.Fatalf("expected two limiter entries, got %d", len(limiter.entries))
	}
	now = now.Add(time.Minute)
	if !limiter.allow("a") {
		t.Fatal("expected a request after the window to be admitted")
	}
}

func TestRateLimitClassLeavesReadsAndRunnerTrafficUnlimited(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/servers"},
		{http.MethodGet, "/api/v1/jobs/next"},
		{http.MethodPost, "/api/v1/jobs/result"},
		{http.MethodPost, "/api/v1/runners/heartbeat"},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if _, limited := rateLimitClass(r); limited {
			t.Errorf("%s %s should not be rate limited", tc.method, tc.path)
		}
	}
	if class, limited := rateLimitClass(httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens", nil)); !limited || class != "auth" {
		t.Fatal("expected auth mutation to use the auth limiter")
	}
}

func TestRequestMiddlewareAddsRequestIDAndLimitsMutations(t *testing.T) {
	limiter := newBoundedRateLimiter(10, 1, time.Minute)
	logger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	handler := requestMiddleware(logger, limiter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	first := httptest.NewRequest(http.MethodPost, "/api/v1/servers", strings.NewReader("{}"))
	first.RemoteAddr = "192.0.2.10:1234"
	first.Header.Set("X-Request-ID", "request-123")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, first)
	if response.Code != http.StatusNoContent || response.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("expected request id and successful first request, got %d and %q", response.Code, response.Header().Get("X-Request-ID"))
	}

	second := httptest.NewRequest(http.MethodPost, "/api/v1/servers", strings.NewReader("{}"))
	second.RemoteAddr = first.RemoteAddr
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, second)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("expected rate limited response, got %d", response.Code)
	}
}
