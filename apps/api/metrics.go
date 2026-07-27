package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	dbruntime "github.com/pgd1001/svrtools/packages/db"
)

// apiMetrics deliberately keeps a small fixed set of counters. Metrics must
// not turn request paths, identities, or query strings into unbounded labels.
type apiMetrics struct {
	requests        atomic.Uint64
	requestFailures atomic.Uint64
	requestDuration atomic.Uint64
	rateLimited     atomic.Uint64
	readinessChecks atomic.Uint64
	readinessFailed atomic.Uint64
}

var metrics apiMetrics

func (m *apiMetrics) observe(status int, duration time.Duration) {
	m.requests.Add(1)
	if status >= http.StatusInternalServerError {
		m.requestFailures.Add(1)
	}
	m.requestDuration.Add(uint64(duration.Milliseconds()))
}

func metricsHandler(w http.ResponseWriter, _ *http.Request) {
	metricsHandlerWithDB(nil)(w, nil)
}

func metricsHandlerWithDB(db *sql.DB, artifactDirs ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "# HELP svrtools_api_requests_total Total HTTP requests handled by the API.\n# TYPE svrtools_api_requests_total counter\nsvrtools_api_requests_total %d\n", metrics.requests.Load())
		fmt.Fprintf(w, "# HELP svrtools_api_request_failures_total HTTP requests that returned a 5xx response.\n# TYPE svrtools_api_request_failures_total counter\nsvrtools_api_request_failures_total %d\n", metrics.requestFailures.Load())
		fmt.Fprintf(w, "# HELP svrtools_api_request_duration_milliseconds_total Sum of request durations in milliseconds.\n# TYPE svrtools_api_request_duration_milliseconds_total counter\nsvrtools_api_request_duration_milliseconds_total %d\n", metrics.requestDuration.Load())
		fmt.Fprintf(w, "# HELP svrtools_api_rate_limited_total Requests rejected by the in-process rate limiter.\n# TYPE svrtools_api_rate_limited_total counter\nsvrtools_api_rate_limited_total %d\n", metrics.rateLimited.Load())
		fmt.Fprintf(w, "# HELP svrtools_api_readiness_checks_total Readiness checks performed.\n# TYPE svrtools_api_readiness_checks_total counter\nsvrtools_api_readiness_checks_total %d\n", metrics.readinessChecks.Load())
		fmt.Fprintf(w, "# HELP svrtools_api_readiness_failures_total Readiness checks that found the database unavailable.\n# TYPE svrtools_api_readiness_failures_total counter\nsvrtools_api_readiness_failures_total %d\n", metrics.readinessFailed.Load())
		if db != nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			pending := queryMetric(ctx, db, "SELECT COUNT(*) FROM execution_targets WHERE status = 'pending'")
			leased := queryMetric(ctx, db, "SELECT COUNT(*) FROM execution_targets WHERE status = 'running' AND lease_id IS NOT NULL")
			deadLetter := queryMetric(ctx, db, "SELECT COUNT(*) FROM execution_targets WHERE status = 'dead_letter'")
			activeRunners := queryMetric(ctx, db, "SELECT COUNT(*) FROM runners WHERE status = 'active' AND last_seen_at >= ?", time.Now().UTC().Add(-2*time.Minute).Format("2006-01-02 15:04:05"))
			enabledSchedules := queryMetric(ctx, db, "SELECT COUNT(*) FROM schedules WHERE enabled = 1")
			fmt.Fprintf(w, "# HELP svrtools_queue_pending_jobs Jobs waiting for a runner.\n# TYPE svrtools_queue_pending_jobs gauge\nsvrtools_queue_pending_jobs %d\n", pending)
			fmt.Fprintf(w, "# HELP svrtools_queue_leased_jobs Jobs currently leased by a runner.\n# TYPE svrtools_queue_leased_jobs gauge\nsvrtools_queue_leased_jobs %d\n", leased)
			fmt.Fprintf(w, "# HELP svrtools_queue_dead_letter_jobs Jobs that exhausted their retry budget.\n# TYPE svrtools_queue_dead_letter_jobs gauge\nsvrtools_queue_dead_letter_jobs %d\n", deadLetter)
			fmt.Fprintf(w, "# HELP svrtools_runners_active Active runners reporting to the control plane.\n# TYPE svrtools_runners_active gauge\nsvrtools_runners_active %d\n", activeRunners)
			fmt.Fprintf(w, "# HELP svrtools_scheduler_enabled_schedules Enabled schedules in the embedded scheduler.\n# TYPE svrtools_scheduler_enabled_schedules gauge\nsvrtools_scheduler_enabled_schedules %d\n", enabledSchedules)
		}
		if len(artifactDirs) > 0 {
			if total, free, ok := diskSpace(artifactDirs[0]); ok && total > 0 {
				fmt.Fprintf(w, "# HELP svrtools_artifact_store_total_bytes Total bytes on the filesystem containing the local artefact store.\n# TYPE svrtools_artifact_store_total_bytes gauge\nsvrtools_artifact_store_total_bytes %d\n", total)
				fmt.Fprintf(w, "# HELP svrtools_artifact_store_free_bytes Free bytes on the filesystem containing the local artefact store.\n# TYPE svrtools_artifact_store_free_bytes gauge\nsvrtools_artifact_store_free_bytes %d\n", free)
				fmt.Fprintf(w, "# HELP svrtools_artifact_store_free_ratio Free-space ratio on the filesystem containing the local artefact store.\n# TYPE svrtools_artifact_store_free_ratio gauge\nsvrtools_artifact_store_free_ratio %.6f\n", float64(free)/float64(total))
			}
		}
	}
}

func apiRuntime() *dbruntime.Runtime { return metadataRuntime() }

func apiExec(ctx context.Context, execer dbruntime.Execer, query string, args ...any) (sql.Result, error) {
	return apiRuntime().ExecContext(ctx, execer, query, args...)
}

func apiQuery(ctx context.Context, queryer dbruntime.Queryer, query string, args ...any) (*sql.Rows, error) {
	return apiRuntime().QueryContext(ctx, queryer, query, args...)
}

func apiQueryRow(ctx context.Context, queryer dbruntime.QueryRower, query string, args ...any) *sql.Row {
	return apiRuntime().QueryRowContext(ctx, queryer, query, args...)
}

func apiCurrentTime() string { return apiRuntime().CurrentTime() }

func queryMetric(ctx context.Context, db *sql.DB, query string, args ...any) int64 {
	var value int64
	if err := apiQueryRow(ctx, db, query, args...).Scan(&value); err != nil {
		return 0
	}
	return value
}
