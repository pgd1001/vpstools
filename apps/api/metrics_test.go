package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMetricsHandlerExposesFixedCounters(t *testing.T) {
	metrics = apiMetrics{}
	metrics.observe(http.StatusOK, 12*time.Millisecond)
	metrics.observe(http.StatusInternalServerError, 4*time.Millisecond)
	metrics.rateLimited.Add(2)
	metrics.readinessChecks.Add(3)

	recorder := httptest.NewRecorder()
	metricsHandler(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain; version=0.0.4") {
		t.Fatalf("metrics content type = %q", got)
	}
	body := recorder.Body.String()
	for _, metric := range []string{
		"svrtools_api_requests_total 2",
		"svrtools_api_request_failures_total 1",
		"svrtools_api_rate_limited_total 2",
		"svrtools_api_readiness_checks_total 3",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics output missing %q:\n%s", metric, body)
		}
	}
}

func TestMetricsHandlerReportsArtifactFilesystemCapacityOnSupportedPlatforms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem capacity probe is implemented for Unix targets")
	}
	root := t.TempDir()
	recorder := httptest.NewRecorder()
	metricsHandlerWithDB(nil, root)(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", recorder.Code)
	}
	for _, metric := range []string{"svrtools_artifact_store_total_bytes", "svrtools_artifact_store_free_bytes", "svrtools_artifact_store_free_ratio"} {
		if !strings.Contains(recorder.Body.String(), metric) {
			t.Fatalf("metrics output missing %q:\n%s", metric, recorder.Body.String())
		}
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("temporary artefact path disappeared: %v", err)
	}
}
