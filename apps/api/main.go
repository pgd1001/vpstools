package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pgd1001/svrtools/packages/ai"
	"github.com/pgd1001/svrtools/packages/artifacts"
	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/config"
	"github.com/pgd1001/svrtools/packages/dispatch"
	"github.com/pgd1001/svrtools/packages/redact"
)

var version = "0.1.0-beta.1"

type tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var policy = authz.NewPolicy()
var apiDB *sql.DB
var apiArtifacts artifacts.Store
var apiBackends config.BackendConfig
var apiDispatcher dispatch.Publisher

type handlerFunc func(w http.ResponseWriter, r *http.Request)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	rateLimiter := newAPIRateLimiter()

	apiBackends = config.Load()
	if err := validateAuthConfig(); err != nil {
		logger.Error("authentication configuration invalid", "error", err)
		os.Exit(1)
	}
	if err := apiBackends.Validate(); err != nil {
		logger.Error("backend configuration invalid", "error", err)
		os.Exit(1)
	}
	if apiBackends.Scheduler != "embedded" || apiBackends.EventBus != "disabled" {
		logger.Error("an unsupported scheduler or event backend was selected", "artifact_store", apiBackends.ArtifactStore, "job_dispatch", apiBackends.JobDispatch, "scheduler", apiBackends.Scheduler, "event_bus", apiBackends.EventBus)
		os.Exit(1)
	}
	db, err := openMetadataDatabase(ctx, apiBackends)
	if err != nil {
		logger.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	apiDB = db
	apiArtifacts, err = newArtifactStore(apiBackends)
	if err != nil {
		logger.Error("artifact store initialisation failed", "error", err)
		os.Exit(1)
	}
	if apiBackends.JobDispatch == "jetstream" {
		natsURL, stream, subject, durable, maxDeliver, ackWait, duplicateWindow := apiBackends.DispatchConfig()
		apiDispatcher, err = dispatch.NewJetStreamPublisher(ctx, dispatch.Config{
			URL: natsURL, Stream: stream, Subject: subject, Durable: durable,
			MaxDeliver: maxDeliver, AckWait: ackWait, DuplicateWindow: duplicateWindow,
		})
		if err != nil {
			logger.Error("JetStream dispatch initialisation failed", "error", err)
			os.Exit(1)
		}
		defer apiDispatcher.Close()
	}

	if err := migrate(ctx, db); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if err := seed(ctx, db); err != nil {
		logger.Error("seed failed", "error", err)
		os.Exit(1)
	}
	if apiBackends.DatabaseDriver == "postgres" && apiBackends.PostgresRLS {
		if err := configurePostgresRLS(ctx, db); err != nil {
			logger.Error("PostgreSQL row-level security configuration failed", "error", err)
			os.Exit(1)
		}
	}
	if apiBackends.AIProvider == "openai-compatible" {
		apiAIProvider = ai.RedactingProvider{Inner: ai.HTTPProvider{Endpoint: apiBackends.AIEndpoint, APIKey: apiBackends.AIAPIKey, Model: apiBackends.AIModel, Timeout: apiBackends.AITimeout, MaxResponse: aiResponseLimit(), HTTPClient: nil}, Redact: redact.Stdout}
	}

	logger.Info("database ready", "tier", apiBackends.Tier(), "database_driver", apiBackends.DatabaseDriver, "artifact_store", apiBackends.ArtifactStore, "job_dispatch", apiBackends.JobDispatch)
	go runEmbeddedScheduler(ctx, db, logger)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "database": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok", "version": version, "deployment_tier": apiBackends.Tier(), "database_driver": apiBackends.DatabaseDriver, "artifact_store": apiBackends.ArtifactStore, "job_dispatch": apiBackends.JobDispatch})
	})
	artifactMetricsDir := ""
	if apiBackends.ArtifactStore == "local" {
		artifactMetricsDir = apiBackends.ArtifactsDir
	}
	mux.HandleFunc("/metrics", metricsHandlerWithDB(db, artifactMetricsDir))
	mux.HandleFunc("/api/v1/ready", func(w http.ResponseWriter, r *http.Request) {
		metrics.readinessChecks.Add(1)
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			metrics.readinessFailed.Add(1)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "database": "unavailable"})
			return
		}
		if apiArtifacts == nil || apiArtifacts.Check() != nil {
			metrics.readinessFailed.Add(1)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "database": "ok", "artifacts": "unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "database": "ok", "artifacts": "ok"})
	})

	mux.HandleFunc("/api/v1/whoami", withAuth(db, handleWhoAmI))
	mux.HandleFunc("/api/v1/ai/analyze", withAuth(db, handleAIAnalyze))
	mux.HandleFunc("/api/v1/auth/tokens", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleCreateAPIToken(w, r)
	}))
	mux.HandleFunc("/api/v1/auth/tokens/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleRevokeAPIToken(w, r, strings.TrimPrefix(r.URL.Path, "/api/v1/auth/tokens/"))
	}))

	mux.HandleFunc("/api/v1/servers", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListServers(w, r)
		case http.MethodPost:
			handleAddServer(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/servers/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/servers/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing server id"})
			return
		}
		if hasSuffix(path, "/check") {
			serverID := path[:len(path)-len("/check")]
			if r.Method == http.MethodPost {
				handleCheckServer(w, r, serverID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetServer(w, r, path)
			return
		}
		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			handleUpdateServer(w, r, path)
			return
		}
		if r.Method == http.MethodDelete {
			handleArchiveServer(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/runners", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			withAuth(db, handleListRunners)(w, r)
		case http.MethodPost:
			handleRegisterRunner(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})

	mux.HandleFunc("/api/v1/runners/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleRunnerHeartbeat(w, r)
	})

	mux.HandleFunc("/api/v1/runners/registration-token", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleCreateRegistrationToken(w, r)
	}))
	mux.HandleFunc("/api/v1/runners/manage", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handleCreateManagedRunner(w, r)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))
	mux.HandleFunc("/api/v1/runners/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/runners/")
		if hasSuffix(path, "/rotate-token") {
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			handleRotateRunnerToken(w, r, strings.TrimSuffix(path, "/rotate-token"))
			return
		}
		id := path
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, 400, map[string]string{"error": "invalid runner id"})
			return
		}
		if r.Method == http.MethodPatch || r.Method == http.MethodPut {
			handleUpdateRunner(w, r, id)
			return
		}
		if r.Method == http.MethodDelete {
			handleRevokeRunner(w, r, id)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/executions", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListExecutions(w, r)
		case http.MethodPost:
			handleCreateExecution(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/executions/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/executions/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing execution id"})
			return
		}
		if hasSuffix(path, "/cancel") {
			execID := path[:len(path)-len("/cancel")]
			if r.Method == http.MethodPost {
				handleCancelExecution(w, r, execID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetExecution(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/jobs/next", func(w http.ResponseWriter, r *http.Request) {
		handleClaimJob(r.Context(), db, w, r)
	})

	mux.HandleFunc("/api/v1/jobs/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		handleSubmitResult(r.Context(), db, w, r)
	})

	mux.HandleFunc("/api/v1/jobs/renew", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleRenewLease(r.Context(), db, w, r)
	})

	mux.HandleFunc("/api/v1/audit", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		handleSearchAudit(w, r)
	}))
	mux.HandleFunc("/api/v1/audit/verify", withAuth(db, handleVerifyAudit))

	mux.HandleFunc("/api/v1/runbooks", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListRunbooks(w, r)
		case http.MethodPost:
			handleCreateRunbook(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/runbooks/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/runbooks/"):]
		if path == "" {
			writeJSON(w, 400, map[string]string{"error": "missing runbook name"})
			return
		}
		if hasSuffix(path, "/run") {
			name := path[:len(path)-len("/run")]
			if r.Method == http.MethodPost {
				handleRunRunbook(w, r, name)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if hasSuffix(path, "/publish") {
			name := path[:len(path)-len("/publish")]
			if r.Method == http.MethodPost {
				handlePublishRunbook(w, r, name)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetRunbook(w, r, path)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			handleUpdateRunbook(w, r, path)
			return
		}
		if r.Method == http.MethodDelete {
			handleArchiveRunbook(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/approvals", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListApprovals(w, r)
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	}))

	mux.HandleFunc("/api/v1/approvals/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/v1/approvals/"):]
		if hasSuffix(path, "/approve") {
			approvalID := path[:len(path)-len("/approve")]
			if r.Method == http.MethodPost {
				handleApprove(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if hasSuffix(path, "/deny") {
			approvalID := path[:len(path)-len("/deny")]
			if r.Method == http.MethodPost {
				handleDeny(w, r, approvalID)
			} else {
				writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			}
			return
		}
		if r.Method == http.MethodGet {
			handleGetApproval(w, r, path)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	}))

	mux.HandleFunc("/api/v1/schedules", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSchedules(w, r)
		case http.MethodPost:
			handleCreateSchedule(w, r)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}))
	mux.HandleFunc("/api/v1/automation/status", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleAutomationStatus(w, r)
	}))
	mux.HandleFunc("/api/v1/automation/pause", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handlePauseAutomation(w, r)
	}))
	mux.HandleFunc("/api/v1/automation/resume", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleResumeAutomation(w, r)
	}))
	mux.HandleFunc("/api/v1/schedules/", withAuth(db, func(w http.ResponseWriter, r *http.Request) {
		scheduleID := strings.TrimPrefix(r.URL.Path, "/api/v1/schedules/")
		if scheduleID == "" || r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		handleDisableSchedule(w, r, scheduleID)
	}))

	addr := ":" + envOrDefault("API_PORT", "8080")
	srv := &http.Server{Addr: addr, Handler: requestMiddleware(logger, rateLimiter, corsMiddleware(mux)), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 1 << 20}

	go func() {
		logger.Info("API listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	srv.Shutdown(shutdownCtx)
}

func newArtifactStore(cfg config.BackendConfig) (artifacts.Store, error) {
	switch cfg.ArtifactStore {
	case "local":
		return artifacts.NewLocalStore(cfg.ArtifactsDir, cfg.ArtifactKey)
	case "s3":
		return artifacts.NewS3Store(cfg.S3Config())
	default:
		return nil, fmt.Errorf("unsupported artifact store %q", cfg.ArtifactStore)
	}
}

type contextKey string

const dbKey contextKey = "db"

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (w *responseRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *responseRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func requestMiddleware(logger *slog.Logger, limiter *boundedRateLimiter, next http.Handler) http.Handler {
	authLimiter := newBoundedRateLimiter(apiRateLimitMaxEntries, apiAuthLimit, apiRateLimitWindow)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if !validRequestID(requestID) {
			requestID = newUUID()
		}
		w.Header().Set("X-Request-ID", requestID)

		if class, limited := rateLimitClass(r); limited {
			limit := limiter
			if class == "auth" {
				limit = authLimiter
			}
			if !limit.allow(class + ":" + rateLimitClient(r)) {
				metrics.rateLimited.Add(1)
				w.Header().Set("Retry-After", strconv.Itoa(int(apiRateLimitWindow/time.Second)))
				writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded", "request_id": requestID})
				return
			}
		}

		started := time.Now()
		rw := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rw, r)
		if rw.status == 0 {
			rw.status = http.StatusOK
		}
		logger.Info("request", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(started).Milliseconds())
		metrics.observe(rw.status, time.Since(started))
	})
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

func withAuth(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer := bearerToken(r); bearer != "" {
			actor, err := resolveAPITokenActor(r.Context(), db, bearer)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired API token"})
				return
			}
			ctx := authz.WithActor(r.Context(), actor)
			ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
				return
			}
			defer cleanup()
			ctx = context.WithValue(ctx, dbKey, db)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			next(w, r.WithContext(ctx))
			return
		}
		sharedSecret := os.Getenv("VPS_WEB_SHARED_SECRET")
		providedSecret := r.Header.Get("X-VPS-Internal-Secret")
		if sharedSecret != "" && secureStringEqual(providedSecret, sharedSecret) {
			actor, err := resolveExternalActor(r.Context(), db, r.Header.Get("X-VPS-OIDC-Subject"), r.Header.Get("X-VPS-OIDC-Email"))
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "OIDC identity is not provisioned"})
				return
			}
			ctx := authz.WithActor(r.Context(), actor)
			ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
				return
			}
			defer cleanup()
			ctx = context.WithValue(ctx, dbKey, db)
			r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			next(w, r.WithContext(ctx))
			return
		}
		if !devAuthEnabled() || productionModeEnabled() {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		userID := r.Header.Get("X-VPS-User")
		if userID == "" {
			userID = "user_senior"
		}
		if userID == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		actor, err := authz.ResolveDevUser(r.Context(), db, userID)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "authentication failed: " + err.Error()})
			return
		}
		ctx := authz.WithActor(r.Context(), actor)
		ctx, cleanup, err := bindTenantConnection(ctx, db, actor.OrganisationID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
			return
		}
		defer cleanup()
		ctx = context.WithValue(ctx, dbKey, db)
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		next(w, r.WithContext(ctx))
	}
}

func devAuthEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("VPS_DEV_AUTH")), "true")
}

func productionModeEnabled() bool {
	for _, key := range []string{"VPS_ENV", "APP_ENV", "ENVIRONMENT"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value == "production" || value == "prod" {
			return true
		}
	}
	return false
}

func secureStringEqual(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) < len("Bearer ") || !strings.EqualFold(value[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(value[len("Bearer "):])
}

func resolveAPITokenActor(ctx context.Context, db *sql.DB, token string) (*authz.Actor, error) {
	if token == "" {
		return nil, fmt.Errorf("API token is empty")
	}
	var actor authz.Actor
	err := apiQueryRow(ctx, db, `
		SELECT u.id, u.email, u.display_name, t.organisation_id, m.role
		FROM api_tokens t
		JOIN users u ON u.id = t.user_id AND u.status = 'active'
		JOIN memberships m ON m.user_id = t.user_id AND m.organisation_id = t.organisation_id AND m.status = 'active'
		WHERE t.token_hash = ? AND t.revoked_at IS NULL AND t.expires_at > `+apiCurrentTime(), hashToken(token)).Scan(
		&actor.UserID, &actor.Email, &actor.DisplayName, &actor.OrganisationID, &actor.Role)
	if err != nil {
		return nil, fmt.Errorf("API token is invalid or expired")
	}
	_, _ = apiExec(ctx, db, "UPDATE api_tokens SET last_used_at = "+apiCurrentTime()+" WHERE token_hash = ? AND revoked_at IS NULL", hashToken(token))
	return &actor, nil
}

func validateAuthConfig() error {
	if !productionModeEnabled() {
		return nil
	}
	if devAuthEnabled() {
		return fmt.Errorf("VPS_DEV_AUTH must be disabled in production")
	}
	secret := os.Getenv("VPS_WEB_SHARED_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("VPS_WEB_SHARED_SECRET must be at least 32 characters in production")
	}
	return nil
}

func dbFrom(r *http.Request) *sql.DB {
	return r.Context().Value(dbKey).(*sql.DB)
}

func dbFromRequest(r *http.Request) *sql.DB {
	if v := r.Context().Value(dbKey); v != nil {
		return v.(*sql.DB)
	}
	return apiDB
}

func authenticateRunnerRegistration(db *sql.DB, r *http.Request) (string, error) {
	orgID, _, err := authenticateRunnerCredential(db, r)
	return orgID, err
}

func authenticateRunnerCredential(db *sql.DB, r *http.Request) (string, string, error) {
	token := runnerToken(r)
	if token == "" && devAuthEnabled() && !productionModeEnabled() {
		return "org_demo", "", nil
	}
	if token == "" {
		return "", "", fmt.Errorf("runner registration credential required")
	}
	var orgID, runnerID string
	err := apiQueryRow(r.Context(), db, `SELECT organisation_id, COALESCE(runner_id,'') FROM runner_credentials WHERE token_hash = ? AND revoked_at IS NULL AND expires_at > `+apiCurrentTime(), hashToken(token)).Scan(&orgID, &runnerID)
	if err != nil {
		return "", "", fmt.Errorf("invalid or expired runner registration credential")
	}
	if runnerID != "" {
		statusRequest := r
		statusContext, cleanup, bindErr := bindTenantConnection(r.Context(), db, orgID)
		if bindErr != nil {
			return "", "", bindErr
		}
		defer cleanup()
		statusRequest = r.WithContext(statusContext)
		var status string
		if err := apiQueryRow(statusRequest.Context(), db, "SELECT status FROM runners WHERE id = ? AND organisation_id = ?", runnerID, orgID).Scan(&status); err != nil || status == "revoked" {
			return "", "", fmt.Errorf("runner credential is no longer valid")
		}
	}
	return orgID, runnerID, nil
}

func authenticateRunner(db *sql.DB, r *http.Request) (string, error) {
	return authenticateRunnerForID(db, r, r.URL.Query().Get("runner_id"))
}

func authenticateRunnerForID(db *sql.DB, r *http.Request, requestedRunnerID string) (string, error) {
	orgID, credentialRunnerID, err := authenticateRunnerCredential(db, r)
	if err != nil {
		return "", err
	}
	if credentialRunnerID != "" && requestedRunnerID != "" && credentialRunnerID != requestedRunnerID {
		return "", fmt.Errorf("runner credential is bound to a different runner")
	}
	return orgID, nil
}

func runnerToken(r *http.Request) string {
	return r.Header.Get("X-VPS-Runner-Token")
}

func handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}
	var orgName string
	apiQueryRow(r.Context(), dbFrom(r), "SELECT name FROM organisations WHERE id = ?", actor.OrganisationID).Scan(&orgName)
	writeJSON(w, 200, map[string]any{
		"user_id":         actor.UserID,
		"email":           actor.Email,
		"name":            actor.DisplayName,
		"organisation_id": actor.OrganisationID,
		"organisation":    orgName,
		"role":            actor.Role,
	})
}

type createAPITokenRequest struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	ExpiresIn int    `json:"expires_in"`
}

func handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if !actor.IsPrivileged() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "API token management requires a privileged role"})
		return
	}
	var req createAPITokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		req.UserID = actor.UserID
	}
	if req.Name == "" {
		req.Name = "cli-token"
	}
	if len(req.Name) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name must be 100 characters or fewer"})
		return
	}
	if req.ExpiresIn == 0 {
		req.ExpiresIn = 30 * 24 * 60 * 60
	}
	if req.ExpiresIn < 60 || req.ExpiresIn > 90*24*60*60 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expires_in must be between 60 seconds and 90 days"})
		return
	}

	var provisionedUser string
	err = apiQueryRow(r.Context(), dbFrom(r), `
		SELECT u.id FROM users u
		JOIN memberships m ON m.user_id = u.id AND m.organisation_id = ? AND m.status = 'active'
		WHERE u.id = ? AND u.status = 'active'`, actor.OrganisationID, req.UserID).Scan(&provisionedUser)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user is not an active member of your organisation"})
		return
	}

	token := newToken()
	if token == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate API token"})
		return
	}
	expiresAt := time.Now().UTC().Add(time.Duration(req.ExpiresIn) * time.Second).Format("2006-01-02 15:04:05")
	tokenID := "pat_" + shortID()
	tokenPrefix := token
	if len(tokenPrefix) > 8 {
		tokenPrefix = tokenPrefix[:8]
	}
	_, err = apiExec(r.Context(), dbFrom(r), `
		INSERT INTO api_tokens (id, organisation_id, user_id, name, token_prefix, token_hash, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, tokenID, actor.OrganisationID, provisionedUser, req.Name, tokenPrefix, hashToken(token), expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create API token"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "auth.token.created", "api_token", tokenID, "success", map[string]any{
		"user_id": provisionedUser, "name": req.Name, "token_prefix": tokenPrefix, "expires_at": expiresAt,
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"token_id": tokenID, "token": token, "user_id": provisionedUser,
		"organisation_id": actor.OrganisationID, "expires_at": expiresAt,
	})
}

func handleRevokeAPIToken(w http.ResponseWriter, r *http.Request, tokenID string) {
	actor, err := authz.RequireActor(r.Context())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	if !actor.IsPrivileged() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "API token management requires a privileged role"})
		return
	}
	if tokenID == "" || strings.Contains(tokenID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token id"})
		return
	}
	result, err := apiExec(r.Context(), dbFrom(r), `
		UPDATE api_tokens SET revoked_at = `+apiCurrentTime()+`
		WHERE id = ? AND organisation_id = ? AND revoked_at IS NULL`, tokenID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to revoke API token"})
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active API token not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "auth.token.revoked", "api_token", tokenID, "success", nil)
	writeJSON(w, http.StatusOK, map[string]string{"token_id": tokenID, "status": "revoked"})
}

func handleListServers(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	envFilter := r.URL.Query().Get("environment")
	tagKey := r.URL.Query().Get("tag_key")
	tagValue := r.URL.Query().Get("tag_value")

	query := `SELECT s.id, s.name, s.hostname, COALESCE(s.public_ip,''), COALESCE(s.private_ip,''),
		s.ssh_port, COALESCE(s.ssh_username,''), s.environment, COALESCE(s.provider,''),
		COALESCE(s.os_name,''), COALESCE(s.os_version,''), COALESCE(s.kernel_version,''),
		COALESCE(s.architecture,''), s.status, COALESCE(s.last_seen_at,''),
		COALESCE(s.last_check_at,''), s.created_at
		FROM servers s WHERE s.organisation_id = ? AND s.status != 'archived'`
	args := []any{actor.OrganisationID}

	if envFilter != "" {
		query += " AND s.environment = ?"
		args = append(args, envFilter)
	}
	if tagKey != "" && tagValue != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?)`
		args = append(args, actor.OrganisationID, tagKey, tagValue)
	} else if tagKey != "" {
		query += ` AND s.id IN (SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ?)`
		args = append(args, actor.OrganisationID, tagKey)
	}

	query += " ORDER BY s.name ASC"

	rows, err := apiQuery(r.Context(), dbFrom(r), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}

	type server struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Hostname    string `json:"hostname"`
		PublicIP    string `json:"public_ip"`
		PrivateIP   string `json:"private_ip"`
		SSHPort     int    `json:"ssh_port"`
		SSHUsername string `json:"ssh_username"`
		Environment string `json:"environment"`
		Provider    string `json:"provider"`
		OSName      string `json:"os_name"`
		OSVersion   string `json:"os_version"`
		Kernel      string `json:"kernel_version"`
		Arch        string `json:"architecture"`
		Status      string `json:"status"`
		LastSeenAt  string `json:"last_seen_at"`
		LastCheckAt string `json:"last_check_at"`
		CreatedAt   string `json:"created_at"`
		Tags        []tag  `json:"tags"`
	}

	servers := []server{}
	for rows.Next() {
		var s server
		if err := rows.Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
			&s.SSHPort, &s.SSHUsername, &s.Environment, &s.Provider,
			&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
			&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	if err := rows.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to close server query"})
		return
	}
	for i := range servers {
		servers[i].Tags = loadTags(r.Context(), dbFrom(r), actor.OrganisationID, servers[i].ID)
	}
	writeJSON(w, 200, map[string]any{"servers": servers})
}

func handleAddServer(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerManagement(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.created", "server", "", dec)
		return
	}

	var req struct {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if req.Environment == "" {
		req.Environment = "development"
	}

	serverID := "srv_" + shortID()

	_, err := apiExec(r.Context(), dbFrom(r),
		`INSERT INTO servers (id, organisation_id, name, hostname, public_ip, private_ip, ssh_port, ssh_username, environment, provider)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		serverID, actor.OrganisationID, req.Name, sqlNullString(req.Hostname), sqlNullString(req.PublicIP),
		sqlNullString(req.PrivateIP), req.SSHPort, sqlNullString(req.SSHUsername), req.Environment, sqlNullString(req.Provider))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create server: " + err.Error()})
		return
	}

	for _, t := range req.Tags {
		if t.Key != "" {
			apiExec(r.Context(), dbFrom(r),
				"INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)",
				actor.OrganisationID, serverID, t.Key, t.Value)
		}
	}

	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "server.created", "server", serverID, "success", map[string]any{"name": req.Name, "environment": req.Environment})

	writeJSON(w, 201, map[string]any{"server_id": serverID, "status": "created"})
}

func handleUpdateServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	if dec := policy.CheckServerManagement(actor); !dec.Allowed {
		writeDenial(w, r, actor, "server.updated", "server", r.URL.Path, dec)
		return
	}
	var req struct {
		Name, Hostname, PublicIP, PrivateIP, SSHUsername, Environment, Provider string
		SSHPort                                                                 int
		Tags                                                                    []tag `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	if !validEnvironment(req.Environment) {
		writeJSON(w, 400, map[string]string{"error": "invalid environment"})
		return
	}
	db := dbFrom(r)
	tx, err := beginAPITx(r.Context(), db)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to start update"})
		return
	}
	defer tx.Rollback()
	res, err := apiExec(r.Context(), tx, `UPDATE servers SET name=?, hostname=?, public_ip=?, private_ip=?, ssh_port=?, ssh_username=?, environment=?, provider=? WHERE id=? AND organisation_id=? AND status != 'archived'`, req.Name, sqlNullString(req.Hostname), sqlNullString(req.PublicIP), sqlNullString(req.PrivateIP), req.SSHPort, sqlNullString(req.SSHUsername), req.Environment, sqlNullString(req.Provider), serverID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update server"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	if _, err = apiExec(r.Context(), tx, "DELETE FROM server_tags WHERE server_id=? AND organisation_id=?", serverID, actor.OrganisationID); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update tags"})
		return
	}
	for _, t := range req.Tags {
		if t.Key != "" {
			if _, err = apiExec(r.Context(), tx, "INSERT INTO server_tags (organisation_id, server_id, key, value) VALUES (?,?,?,?)", actor.OrganisationID, serverID, t.Key, t.Value); err != nil {
				writeJSON(w, 500, map[string]string{"error": "failed to update tags"})
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to commit update"})
		return
	}
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "server.updated", "server", serverID, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func handleArchiveServer(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if dec := policy.CheckServerManagement(actor); !dec.Allowed {
		writeDenial(w, r, actor, "server.archived", "server", r.URL.Path, dec)
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE servers SET status='archived' WHERE id=? AND organisation_id=? AND status != 'archived'", id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to archive server"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "server.archived", "server", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "archived"})
}

type serverDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Hostname    string `json:"hostname"`
	PublicIP    string `json:"public_ip"`
	PrivateIP   string `json:"private_ip"`
	SSHPort     int    `json:"ssh_port"`
	SSHUsername string `json:"ssh_username"`
	Environment string `json:"environment"`
	Provider    string `json:"provider"`
	OSName      string `json:"os_name"`
	OSVersion   string `json:"os_version"`
	Kernel      string `json:"kernel_version"`
	Arch        string `json:"architecture"`
	Status      string `json:"status"`
	LastSeenAt  string `json:"last_seen_at"`
	LastCheckAt string `json:"last_check_at"`
	CreatedAt   string `json:"created_at"`
	Tags        []tag  `json:"tags"`
}

func handleGetServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	s := serverDetail{
		Tags: loadTags(r.Context(), db, actor.OrganisationID, serverID),
	}

	err := apiQueryRow(r.Context(), db,
		`SELECT id, name, COALESCE(hostname,''), COALESCE(public_ip,''), COALESCE(private_ip,''),
		ssh_port, COALESCE(ssh_username,''), environment, COALESCE(provider,''),
		COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''),
		COALESCE(architecture,''), status, COALESCE(last_seen_at,''),
		COALESCE(last_check_at,''), created_at
		FROM servers WHERE id = ? AND organisation_id = ?`, serverID, actor.OrganisationID,
	).Scan(&s.ID, &s.Name, &s.Hostname, &s.PublicIP, &s.PrivateIP,
		&s.SSHPort, &s.SSHUsername, &s.Environment, &s.Provider,
		&s.OSName, &s.OSVersion, &s.Kernel, &s.Arch,
		&s.Status, &s.LastSeenAt, &s.LastCheckAt, &s.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"server": s})
}

func handleCheckServer(w http.ResponseWriter, r *http.Request, serverID string) {
	actor, _ := authz.RequireActor(r.Context())
	dec := policy.CheckServerCheck(actor)
	if !dec.Allowed {
		writeDenial(w, r, actor, "server.checked", "server", serverID, dec)
		return
	}

	db := dbFrom(r)
	var host, sshUser string
	var sshPort int
	err := apiQueryRow(r.Context(), db,
		"SELECT COALESCE(hostname, public_ip, ''), ssh_port, COALESCE(ssh_username,'') FROM servers WHERE id = ? AND organisation_id = ?",
		serverID, actor.OrganisationID).Scan(&host, &sshPort, &sshUser)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "server not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := apiExec(r.Context(), db,
		"UPDATE servers SET status = 'active', last_check_at = ?, last_seen_at = ? WHERE id = ?",
		now, now, serverID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update server check"})
		return
	}

	checkResult := map[string]any{
		"server_id":  serverID,
		"status":     "reachable",
		"hostname":   host,
		"ssh_port":   sshPort,
		"checked_at": now,
	}

	var osName, osVer, kernel, arch string
	apiQueryRow(r.Context(), db,
		"SELECT COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(kernel_version,''), COALESCE(architecture,'') FROM servers WHERE id = ?",
		serverID).Scan(&osName, &osVer, &kernel, &arch)

	if osName == "" {
		osName = "linux"
		osVer = "unknown"
		kernel = "unknown"
		arch = "amd64"
		apiExec(r.Context(), db,
			"UPDATE servers SET os_name=?, os_version=?, kernel_version=?, architecture=? WHERE id=?",
			osName, osVer, kernel, arch, serverID)
	}
	checkResult["os_name"] = osName
	checkResult["os_version"] = osVer
	checkResult["kernel_version"] = kernel
	checkResult["architecture"] = arch
	checkResult["uptime"] = "0d 0h 0m (simulated)"

	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "server.checked", "server", serverID, "success", nil)
	writeJSON(w, 200, map[string]any{"server": checkResult})
}

func handleListRunners(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	rows, err := apiQuery(r.Context(), dbFrom(r),
		`SELECT id, name, runner_type, status, COALESCE(version,''), COALESCE(hostname,''),
		COALESCE(platform,''), COALESCE(ip_address,''), COALESCE(last_seen_at,''),
		COALESCE(registered_at,''), COALESCE(revoked_at,''), created_at
		FROM runners WHERE organisation_id = ? ORDER BY name ASC`, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type runner struct {
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

	results := []runner{}
	for rows.Next() {
		var rn runner
		rows.Scan(&rn.ID, &rn.Name, &rn.RunnerType, &rn.Status, &rn.Version,
			&rn.Hostname, &rn.Platform, &rn.IPAddress, &rn.LastSeenAt,
			&rn.RegisteredAt, &rn.RevokedAt, &rn.CreatedAt)
		results = append(results, rn)
	}
	writeJSON(w, 200, map[string]any{"runners": results})
}

func handleCreateManagedRunner(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.created", "runner", "", authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	var req struct {
		Name       string `json:"name"`
		RunnerType string `json:"runner_type"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.RunnerType == "" {
		req.RunnerType = "customer_managed"
	}
	id := "rnr_" + shortID()
	_, err := apiExec(r.Context(), dbFrom(r), `INSERT INTO runners (id, organisation_id, name, runner_type, status) VALUES (?,?,?,?, 'pending')`, id, actor.OrganisationID, req.Name, req.RunnerType)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to create runner"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.created", "runner", id, "success", map[string]any{"name": req.Name})
	writeJSON(w, 201, map[string]string{"runner_id": id, "status": "pending"})
}

func handleUpdateRunner(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.updated", "runner", id, authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	var req struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status != "active" && req.Status != "paused" {
		writeJSON(w, 400, map[string]string{"error": "invalid status"})
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE runners SET name=?, status=? WHERE id=? AND organisation_id=? AND status != 'revoked'", req.Name, req.Status, id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to update runner"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.updated", "runner", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func handleRevokeRunner(w http.ResponseWriter, r *http.Request, id string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.revoked", "runner", id, authz.Deny("runner_management_required", "Runner management requires a privileged role."))
		return
	}
	res, err := apiExec(r.Context(), dbFrom(r), "UPDATE runners SET status='revoked', revoked_at="+apiCurrentTime()+" WHERE id=? AND organisation_id=? AND status != 'revoked'", id, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to revoke runner"})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	if _, err := apiExec(r.Context(), dbFrom(r), "UPDATE runner_credentials SET revoked_at = "+apiCurrentTime()+" WHERE runner_id = ? AND revoked_at IS NULL", id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runner revoked but credential revocation failed"})
		return
	}
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, "runner.revoked", "runner", id, "success", nil)
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}

func handleRegisterRunner(w http.ResponseWriter, r *http.Request) {
	orgID, credentialRunnerID, err := authenticateRunnerCredential(dbFromRequest(r), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		Hostname   string `json:"hostname"`
		Platform   string `json:"platform"`
		IPAddress  string `json:"ip_address"`
		RunnerType string `json:"runner_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}
	if req.RunnerType == "" {
		req.RunnerType = "customer_managed"
	}

	runnerID := "rnr_" + shortID()

	db := dbFromRequest(r)
	if credentialRunnerID != "" {
		now := time.Now().UTC()
		result, updateErr := apiExec(r.Context(), db,
			`UPDATE runners SET status='active', version=?, hostname=?, platform=?, ip_address=?, registered_at=COALESCE(registered_at,?), last_seen_at=?, updated_at=? WHERE id=? AND organisation_id=? AND status != 'revoked'`,
			sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform), sqlNullString(req.IPAddress), now, now, now, credentialRunnerID, orgID)
		if updateErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to register runner"})
			return
		}
		if n, _ := result.RowsAffected(); n == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "runner is not available for registration"})
			return
		}
		runnerID = credentialRunnerID
	} else {
		now := time.Now().UTC()
		_, err = apiExec(r.Context(), db,
			`INSERT INTO runners (id, organisation_id, name, runner_type, status, version, hostname, platform, ip_address, registered_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			runnerID, orgID, req.Name, req.RunnerType, "active",
			sqlNullString(req.Version), sqlNullString(req.Hostname), sqlNullString(req.Platform),
			sqlNullString(req.IPAddress), now)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to register runner: " + err.Error()})
			return
		}
	}

	if _, err := apiExec(r.Context(), db,
		"INSERT INTO runner_scopes (id, organisation_id, runner_id, scope_type, scope_value) VALUES (?,?,?,?,?) ON CONFLICT DO NOTHING",
		"rsc_"+shortID(), orgID, runnerID, "all", "*"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to assign runner scope"})
		return
	}

	writeAuditEvent(r.Context(), db, orgID, "", "runner.registered", "runner", runnerID, "success", map[string]any{"name": req.Name})
	writeJSON(w, 201, map[string]any{"runner_id": runnerID, "organisation_id": orgID, "status": "active"})
}

func handleRunnerHeartbeat(w http.ResponseWriter, r *http.Request) {
	orgID, err := authenticateRunner(dbFromRequest(r), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	var req struct {
		RunnerID string `json:"runner_id"`
		Hostname string `json:"hostname"`
		Platform string `json:"platform"`
		Version  string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if _, err := authenticateRunnerForID(dbFromRequest(r), r, req.RunnerID); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	result, err := apiExec(r.Context(), dbFromRequest(r),
		"UPDATE runners SET status = 'active', last_seen_at = ?, hostname = COALESCE(NULLIF(?,''), hostname), platform = COALESCE(NULLIF(?,''), platform), version = COALESCE(NULLIF(?,''), version) WHERE id = ? AND organisation_id = ?",
		now, req.Hostname, req.Platform, req.Version, req.RunnerID, orgID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "runner not found"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleCreateRegistrationToken(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.credential.rotated", "runner", "", authz.Deny("runner_management_required", "Creating runner credentials requires a privileged role."))
		return
	}
	var req struct {
		RunnerID string `json:"runner_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
	}
	token, err := createRunnerCredential(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, req.RunnerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{
		"registration_token": token,
		"expires_in":         "3600",
	})
}

func createRunnerCredential(ctx context.Context, db *sql.DB, orgID, actorID, runnerID string) (string, error) {
	if runnerID != "" {
		var status string
		if err := apiQueryRow(ctx, db, "SELECT status FROM runners WHERE id = ? AND organisation_id = ?", runnerID, orgID).Scan(&status); err != nil {
			return "", fmt.Errorf("runner not found")
		}
		if status == "revoked" {
			return "", fmt.Errorf("runner is revoked")
		}
		if _, err := apiExec(ctx, db, "UPDATE runner_credentials SET revoked_at = "+apiCurrentTime()+" WHERE runner_id = ? AND revoked_at IS NULL", runnerID); err != nil {
			return "", fmt.Errorf("failed to revoke previous runner credentials")
		}
	}
	token := newToken()
	if token == "" {
		return "", fmt.Errorf("failed to generate registration token")
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	if _, err := apiExec(ctx, db, "INSERT INTO runner_credentials (id, organisation_id, runner_id, token_hash, expires_at) VALUES (?,?,?,?,?)", "rct_"+shortID(), orgID, sqlNullString(runnerID), hashToken(token), expiresAt); err != nil {
		return "", fmt.Errorf("failed to create registration token")
	}
	writeAuditEvent(ctx, db, orgID, actorID, "runner.credential.rotated", "runner", runnerID, "success", map[string]any{"expires_in": 3600})
	return token, nil
}

func handleRotateRunnerToken(w http.ResponseWriter, r *http.Request, runnerID string) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.CanManageRunners() {
		writeDenial(w, r, actor, "runner.credential.rotated", "runner", runnerID, authz.Deny("runner_management_required", "Rotating runner credentials requires a privileged role."))
		return
	}
	if runnerID == "" || strings.Contains(runnerID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid runner id"})
		return
	}
	token, err := createRunnerCredential(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, runnerID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"registration_token": token, "expires_in": "3600", "runner_id": runnerID})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" && origin == envOrDefault("VPS_WEB_ORIGIN", "http://localhost:3000") {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-VPS-User, X-VPS-Runner-Token, X-VPS-Internal-Secret, X-VPS-OIDC-Subject, X-VPS-OIDC-Email")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleListExecutions(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	status := r.URL.Query().Get("status")
	mineFilter := r.URL.Query().Get("mine")
	delegatedBy := r.URL.Query().Get("delegated_by")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}

	query := `SELECT e.id, e.actor_user_id, e.actor_role_at_time, e.execution_type, e.status,
		e.risk_level, e.environment, e.reason, e.command_preview, e.command_hash,
		e.timeout_seconds, e.requested_at, e.started_at, e.finished_at,
		COALESCE(e.delegated_by_user_id,''), COALESCE(e.approval_id,''),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id AND et.status = 'succeeded'),
		(SELECT COUNT(*) FROM execution_targets et WHERE et.execution_id = e.id AND et.status = 'failed')
		FROM executions e WHERE e.organisation_id = ?`
	args := []any{actor.OrganisationID}
	if status != "" {
		query += " AND e.status = ?"
		args = append(args, status)
	}
	if mineFilter == "true" || mineFilter == "1" {
		query += " AND e.actor_user_id = ?"
		args = append(args, actor.UserID)
	}
	if delegatedBy != "" {
		query += " AND e.delegated_by_user_id = ?"
		args = append(args, delegatedBy)
	}
	query += " ORDER BY e.requested_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := apiQuery(r.Context(), dbFrom(r), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type execution struct {
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
		TargetCount    int    `json:"target_count"`
		SucceededCount int    `json:"succeeded_count"`
		FailedCount    int    `json:"failed_count"`
		DelegatedBy    string `json:"delegated_by_user_id"`
		ApprovalID     string `json:"approval_id"`
	}
	var results []execution
	for rows.Next() {
		var e execution
		rows.Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
			&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
			&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt,
			&e.DelegatedBy, &e.ApprovalID, &e.TargetCount, &e.SucceededCount, &e.FailedCount)
		results = append(results, e)
	}
	writeJSON(w, 200, map[string]any{"executions": results})
}

func handleGetExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	var e struct {
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
	err := apiQueryRow(r.Context(), db,
		`SELECT id, actor_user_id, actor_role_at_time, execution_type, status,
		risk_level, environment, reason, command_preview, command_hash,
		timeout_seconds, COALESCE(requested_at,''), COALESCE(started_at,''), COALESCE(finished_at,''),
		COALESCE(error_summary,''), COALESCE(delegated_by_user_id,''), COALESCE(approval_id,'')
		FROM executions WHERE id = ? AND organisation_id = ?`, execID, actor.OrganisationID,
	).Scan(&e.ID, &e.ActorUserID, &e.ActorRole, &e.ExecutionType, &e.Status,
		&e.RiskLevel, &e.Environment, &e.Reason, &e.CommandPreview, &e.CommandHash,
		&e.TimeoutSeconds, &e.RequestedAt, &e.StartedAt, &e.FinishedAt, &e.ErrorSummary,
		&e.DelegatedBy, &e.ApprovalID)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "execution not found"})
		return
	}

	type targetResult struct {
		ID               string `json:"id"`
		ServerID         string `json:"server_id"`
		RunnerID         string `json:"runner_id"`
		Status           string `json:"status"`
		ExitCode         int    `json:"exit_code"`
		Stdout           string `json:"stdout"`
		Stderr           string `json:"stderr"`
		StartedAt        string `json:"started_at"`
		FinishedAt       string `json:"finished_at"`
		Error            string `json:"error_summary"`
		StdoutArtifactID string `json:"-"`
		StderrArtifactID string `json:"-"`
	}
	rows, err := apiQuery(r.Context(), db,
		`SELECT id, server_id, COALESCE(runner_id,''), status, COALESCE(exit_code,0),
		stdout, stderr, COALESCE(stdout_artifact_id,''), COALESCE(stderr_artifact_id,''), COALESCE(started_at,''), COALESCE(finished_at,''), COALESCE(error_summary,'')
		FROM execution_targets WHERE execution_id = ? ORDER BY server_id`, execID)
	var targets []targetResult
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t targetResult
			rows.Scan(&t.ID, &t.ServerID, &t.RunnerID, &t.Status,
				&t.ExitCode, &t.Stdout, &t.Stderr, &t.StdoutArtifactID, &t.StderrArtifactID, &t.StartedAt, &t.FinishedAt, &t.Error)
			if t.StdoutArtifactID != "" && apiArtifacts != nil {
				if data, _, artifactErr := apiArtifacts.Get(t.StdoutArtifactID); artifactErr == nil {
					t.Stdout = string(data)
				}
			}
			if t.StderrArtifactID != "" && apiArtifacts != nil {
				if data, _, artifactErr := apiArtifacts.Get(t.StderrArtifactID); artifactErr == nil {
					t.Stderr = string(data)
				}
			}
			targets = append(targets, t)
		}
	}
	type executionEvent struct {
		ID         string `json:"id"`
		TargetID   string `json:"target_id"`
		FromStatus string `json:"from_status"`
		ToStatus   string `json:"to_status"`
		EventType  string `json:"event_type"`
		Metadata   string `json:"metadata"`
		OccurredAt string `json:"occurred_at"`
	}
	eventRows, err := apiQuery(r.Context(), db, `SELECT id, COALESCE(target_id,''), COALESCE(from_status,''), to_status, event_type, metadata, occurred_at FROM execution_events WHERE execution_id = ? AND organisation_id = ? ORDER BY occurred_at ASC, id ASC`, execID, actor.OrganisationID)
	var events []executionEvent
	if err == nil {
		defer eventRows.Close()
		for eventRows.Next() {
			var event executionEvent
			if eventRows.Scan(&event.ID, &event.TargetID, &event.FromStatus, &event.ToStatus, &event.EventType, &event.Metadata, &event.OccurredAt) == nil {
				events = append(events, event)
			}
		}
	}

	writeJSON(w, 200, map[string]any{"execution": e, "targets": targets, "events": events})
}

func handleCreateExecution(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)

	var req struct {
		Target      string `json:"target"`
		Command     string `json:"command"`
		Reason      string `json:"reason"`
		DelegatedBy string `json:"delegated_by_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Target == "" || req.Command == "" {
		writeJSON(w, 400, map[string]string{"error": "target and command are required"})
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey != "" && !validIdempotencyKey(idempotencyKey) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key must contain only letters, numbers, '.', '_' or '-' and be at most 128 characters"})
		return
	}
	payloadHash := hashPayload(req)
	if idempotencyKey != "" {
		if replayed, err := replayIdempotentExecution(r.Context(), db, actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, w); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect idempotency key"})
			return
		} else if replayed {
			return
		}
	}

	targetIDs := resolveTargets(r.Context(), db, actor.OrganisationID, req.Target)
	if len(targetIDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no servers found for target: " + req.Target})
		return
	}

	env, mixed := targetEnvironment(r.Context(), db, actor.OrganisationID, targetIDs)
	if mixed {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "targets span multiple environments; submit one environment at a time"})
		return
	}
	risk := authz.ClassifyRisk(req.Command)

	dec := policy.CheckExecution(r.Context(), db, actor, authz.Env(env), risk, req.Reason)
	if !dec.Allowed {
		writeDenial(w, r, actor, "execution.requested", "execution", req.Target, dec)
		return
	}

	execID := "exe_" + shortID()

	targetSnapshot := snapshotTargets(r.Context(), db, actor.OrganisationID, targetIDs)
	if len(targetSnapshot) != len(targetIDs) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to snapshot selected targets"})
		return
	}
	tx, err := beginAPITx(r.Context(), db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start execution transaction"})
		return
	}
	defer tx.Rollback()
	_, err = apiExec(r.Context(), tx,
		`INSERT INTO executions (id, organisation_id, actor_user_id, actor_role_at_time, delegated_by_user_id, execution_type, status, environment, risk_level, command, command_preview, command_hash, reason, timeout_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		execID, actor.OrganisationID, actor.UserID, actor.Role, sqlNullString(req.DelegatedBy), "raw_command", "queued", env, string(risk), req.Command, redact.Stdout(req.Command), hashCmd(req.Command), req.Reason, 300,
	)
	if err != nil {
		slog.Error("execution create error", "error", err)
		writeJSON(w, 500, map[string]string{"error": "failed to create execution"})
		return
	}

	for i, srvID := range targetIDs {
		if _, err = apiExec(r.Context(), tx,
			`INSERT INTO execution_targets (id, organisation_id, execution_id, server_id, status, server_snapshot)
			VALUES (?, ?, ?, ?, 'pending', `+metadataRuntime().JSONParameter()+`)`,
			"ext_"+shortID(), actor.OrganisationID, execID, srvID, jsonString(targetSnapshot[i])); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create execution target"})
			return
		}
	}
	if err = recordExecutionEvent(r.Context(), tx, actor.OrganisationID, execID, "", "", "queued", "execution.queued", map[string]any{"execution_type": "raw_command"}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if err = writeAuditEventTx(r.Context(), tx, actor.OrganisationID, actor.UserID, "execution.requested", "execution", execID, "queued", map[string]any{
		"command": redact.Stdout(req.Command), "reason": req.Reason, "target": req.Target, "risk": string(risk), "target_count": len(targetIDs),
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution audit event"})
		return
	}
	responseBody := jsonString(map[string]any{
		"execution_id": execID,
		"status":       "queued",
		"risk_level":   string(risk),
		"target_count": len(targetIDs),
	})
	if idempotencyKey != "" {
		if _, err = apiExec(r.Context(), tx,
			`INSERT INTO execution_idempotency (organisation_id, user_id, idempotency_key, payload_hash, execution_id, response_body) VALUES (?,?,?,?,?,?)`,
			actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, execID, responseBody); err != nil {
			_ = tx.Rollback()
			if replayed, replayErr := replayIdempotentExecution(r.Context(), db, actor.OrganisationID, actor.UserID, idempotencyKey, payloadHash, w); replayErr == nil && replayed {
				return
			}
			writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key is already in use"})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit execution"})
		return
	}
	if err := publishPendingJobNotifications(r.Context(), db, execID); err != nil {
		slog.Warn("execution committed but JetStream notification publication was incomplete", "execution_id", execID, "error", err)
	}

	writeRawJSON(w, http.StatusCreated, responseBody)
}

func replayIdempotentExecution(ctx context.Context, db *sql.DB, orgID, userID, key, payloadHash string, w http.ResponseWriter) (bool, error) {
	var storedHash, responseBody string
	err := apiQueryRow(ctx, db,
		`SELECT payload_hash, response_body FROM execution_idempotency WHERE organisation_id = ? AND user_id = ? AND idempotency_key = ?`,
		orgID, userID, key).Scan(&storedHash, &responseBody)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedHash != payloadHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key was already used with a different request payload"})
		return true, nil
	}
	w.Header().Set("Idempotency-Replayed", "true")
	writeRawJSON(w, http.StatusCreated, responseBody)
	return true, nil
}

func handleCancelExecution(w http.ResponseWriter, r *http.Request, execID string) {
	actor, _ := authz.RequireActor(r.Context())
	db := dbFrom(r)
	var requesterID string
	if err := apiQueryRow(r.Context(), db,
		"SELECT actor_user_id FROM executions WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		execID, actor.OrganisationID).Scan(&requesterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "execution not cancellable"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution"})
		return
	}
	if actor.UserID != requesterID && !canManageExecution(actor.Role) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only the requester or a senior operator can cancel this execution"})
		return
	}
	result, err := apiExec(r.Context(), db,
		"UPDATE executions SET status = 'cancelled', finished_at = ? WHERE id = ? AND organisation_id = ? AND status IN ('created','queued')",
		time.Now().UTC(), execID, actor.OrganisationID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to cancel"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 400, map[string]string{"error": "execution not cancellable"})
		return
	}
	_, _ = apiExec(r.Context(), db, "UPDATE execution_targets SET status = 'cancelled' WHERE execution_id = ?", execID)
	writeAuditEvent(r.Context(), db, actor.OrganisationID, actor.UserID, "execution.cancelled", "execution", execID, "cancelled", nil)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func canManageExecution(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "owner", "admin", "senior_engineer":
		return true
	default:
		return false
	}
}

func handleSearchAudit(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	actorFilter := r.URL.Query().Get("actor")
	limit := "20"
	if l := r.URL.Query().Get("limit"); l != "" {
		limit = l
	}
	query := "SELECT id, organisation_id, actor_user_id, action, target_type, target_id, result, metadata, occurred_at FROM audit_events WHERE organisation_id = ?"
	args := []any{actor.OrganisationID}
	if actorFilter != "" {
		query += " AND actor_user_id = ?"
		args = append(args, actorFilter)
	}
	query += " ORDER BY occurred_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := apiQuery(r.Context(), dbFrom(r), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var id, orgID, actorID, action, targetType, targetID, result, metadata, createdAt string
		if err := rows.Scan(&id, &orgID, &actorID, &action, &targetType, &targetID, &result, &metadata, &createdAt); err != nil {
			continue
		}
		events = append(events, map[string]any{
			"id":              id,
			"organisation_id": orgID,
			"actor_id":        actorID,
			"action":          action,
			"target_type":     targetType,
			"target_id":       targetID,
			"result":          result,
			"metadata":        metadata,
			"created_at":      createdAt,
		})
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func handleVerifyAudit(w http.ResponseWriter, r *http.Request) {
	actor, _ := authz.RequireActor(r.Context())
	if !actor.IsSenior() && strings.ToLower(actor.Role) != "auditor" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "audit verification requires an auditor or senior operator"})
		return
	}
	checked, err := verifyAuditHashChain(r.Context(), dbFrom(r), actor.OrganisationID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"valid": false, "checked_events": checked, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "checked_events": checked})
}

func writeDenial(w http.ResponseWriter, r *http.Request, actor *authz.Actor, action, targetType, targetID string, dec authz.Decision) {
	writeAuditEvent(r.Context(), dbFrom(r), actor.OrganisationID, actor.UserID, action, targetType, targetID, "denied", map[string]any{
		"reason":  dec.Reason,
		"message": dec.Message,
	})
	writeJSON(w, 403, map[string]string{
		"error":  dec.Message,
		"reason": dec.Reason,
		"next":   "Contact your admin or request approval if available.",
	})
}

func resolveTargets(ctx context.Context, db *sql.DB, orgID, target string) []string {
	if strings.HasPrefix(target, "server:") {
		serverID := target[len("server:"):]
		var exists string
		err := apiQueryRow(ctx, db, "SELECT id FROM servers WHERE (id = ? OR name = ?) AND organisation_id = ? AND status != 'archived'",
			serverID, serverID, orgID).Scan(&exists)
		if err != nil {
			return nil
		}
		return []string{exists}
	}
	if strings.HasPrefix(target, "tag:") {
		parts := strings.SplitN(target[len("tag:"):], "=", 2)
		if len(parts) != 2 {
			return nil
		}
		rows, err := apiQuery(ctx, db,
			"SELECT server_id FROM server_tags WHERE organisation_id = ? AND key = ? AND value = ?", orgID, parts[0], parts[1])
		if err != nil {
			return nil
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			rows.Scan(&id)
			ids = append(ids, id)
		}
		return ids
	}
	var exists string
	err := apiQueryRow(ctx, db, "SELECT id FROM servers WHERE name = ? AND organisation_id = ? AND status != 'archived'",
		target, orgID).Scan(&exists)
	if err != nil {
		return nil
	}
	return []string{exists}
}

func detectEnv(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) string {
	env, _ := targetEnvironment(ctx, db, orgID, serverIDs)
	return env
}

func targetEnvironment(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) (string, bool) {
	if len(serverIDs) == 0 {
		return "", false
	}
	environments := make(map[string]struct{})
	for _, serverID := range serverIDs {
		var env string
		if err := apiQueryRow(ctx, db, "SELECT environment FROM servers WHERE id = ? AND organisation_id = ?", serverID, orgID).Scan(&env); err != nil {
			return "", true
		}
		environments[env] = struct{}{}
	}
	if len(environments) != 1 {
		return "", true
	}
	for env := range environments {
		return env, false
	}
	return "", true
}

func snapshotTargets(ctx context.Context, db *sql.DB, orgID string, serverIDs []string) []map[string]any {
	snapshots := make([]map[string]any, 0, len(serverIDs))
	for _, serverID := range serverIDs {
		var id, name, hostname, environment, status string
		var port int
		if err := apiQueryRow(ctx, db,
			"SELECT id, name, COALESCE(hostname,''), environment, status, ssh_port FROM servers WHERE id = ? AND organisation_id = ?",
			serverID, orgID).Scan(&id, &name, &hostname, &environment, &status, &port); err != nil {
			return nil
		}
		snapshots = append(snapshots, map[string]any{
			"id": id, "name": name, "hostname": hostname, "environment": environment,
			"status": status, "ssh_port": port,
		})
	}
	return snapshots
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func sqlNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

func loadTags(ctx context.Context, db *sql.DB, orgID, serverID string) []tag {
	rows, err := apiQuery(ctx, db,
		"SELECT key, value FROM server_tags WHERE organisation_id = ? AND server_id = ? ORDER BY key", orgID, serverID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var tags []tag
	for rows.Next() {
		var t tag
		if err := rows.Scan(&t.Key, &t.Value); err != nil {
			continue
		}
		tags = append(tags, t)
	}
	return tags
}

func handleClaimJob(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	runtime := metadataRuntime()
	runnerOrg, err := authenticateRunner(db, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, runnerOrg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	runnerID := r.URL.Query().Get("runner_id")
	if runnerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runner_id is required"})
		return
	}
	targetFilter := r.URL.Query().Get("target_id")
	tx, err := beginAPITx(ctx, db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start job claim"})
		return
	}
	defer tx.Rollback()
	if err := reconcileExpiredLeases(ctx, tx, runnerOrg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reconcile expired jobs"})
		return
	}
	var targetID, execID, command, host, sshUser, sourceStatus string
	var sshPort, timeout int
	err = runtime.QueryRowContext(ctx, tx, `SELECT et.id, e.id, et.status, COALESCE(NULLIF(e.command,''), e.command_preview), COALESCE(s.hostname, s.public_ip, ''), s.ssh_port, COALESCE(s.ssh_username,''), e.timeout_seconds
		FROM execution_targets et JOIN executions e ON e.id = et.execution_id JOIN servers s ON s.id = et.server_id
		WHERE e.status IN ('queued','running') AND e.organisation_id = ?
		AND (? = '' OR et.id = ?)
		AND ((et.status = 'pending' AND (et.next_attempt_at IS NULL OR et.next_attempt_at <= `+runtime.CurrentTime()+`))
			OR (et.status = 'running' AND et.lease_expires_at IS NOT NULL AND et.lease_expires_at <= `+runtime.CurrentTime()+`))
		AND et.attempt < et.max_attempts
		AND EXISTS (SELECT 1 FROM runners rn JOIN runner_scopes rs ON rs.runner_id = rn.id
			WHERE rn.id = ? AND rn.organisation_id = ? AND rn.status = 'active'
			AND (rs.scope_type = 'all' OR (rs.scope_type = 'server' AND rs.scope_value = et.server_id)))
		ORDER BY e.requested_at ASC LIMIT 1`, runnerOrg, targetFilter, targetFilter, runnerID, runnerOrg).Scan(&targetID, &execID, &sourceStatus, &command, &host, &sshPort, &sshUser, &timeout)
	if err != nil {
		if err := tx.Commit(); err != nil {
			_ = err
		}
		writeJSON(w, 404, map[string]string{"status": "no_jobs"})
		return
	}
	leaseID := "lease_" + shortID()
	leaseExpires := time.Now().UTC().Add(5 * time.Minute)
	claimResult, err := runtime.ExecContext(ctx, tx, "UPDATE execution_targets SET status = 'running', runner_id = ?, lease_id = ?, lease_expires_at = ?, attempt = attempt + 1, started_at = COALESCE(started_at, "+runtime.CurrentTime()+") WHERE id = ? AND status = ? AND attempt < max_attempts AND ((status = 'pending' AND (next_attempt_at IS NULL OR next_attempt_at <= "+runtime.CurrentTime()+")) OR (status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= "+runtime.CurrentTime()+"))", runnerID, leaseID, leaseExpires, targetID, sourceStatus)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to claim job"})
		return
	}
	if affected, _ := claimResult.RowsAffected(); affected != 1 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job was claimed by another runner"})
		return
	}
	if _, err = runtime.ExecContext(ctx, tx, "UPDATE executions SET status = 'running', started_at = COALESCE(started_at, "+runtime.CurrentTime()+") WHERE id = ? AND status IN ('queued','running')", execID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "job was claimed by another runner"})
		return
	}
	if err = recordExecutionEvent(ctx, tx, runnerOrg, execID, targetID, sourceStatus, "running", "execution.started", map[string]any{"runner_id": runnerID, "lease_id": leaseID}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if err = tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit job claim"})
		return
	}

	writeJSON(w, 200, map[string]any{
		"target_id":    targetID,
		"execution_id": execID,
		"command":      command,
		"host":         host,
		"port":         sshPort,
		"user":         sshUser,
		"timeout":      timeout,
		"lease_id":     leaseID,
	})
}

func handleSubmitResult(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	orgID, err := authenticateRunner(db, r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var req struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
		ExitCode    int    `json:"exit_code"`
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		Error       string `json:"error"`
		DurationMs  int64  `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExecutionID == "" || req.TargetID == "" || req.RunnerID == "" || req.LeaseID == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if _, err := authenticateRunnerForID(db, r, req.RunnerID); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	req.Stdout = redact.Stdout(req.Stdout)
	req.Stderr = redact.Stdout(req.Stderr)
	status := "succeeded"
	if req.ExitCode != 0 || req.Error != "" {
		status = "failed"
	}
	payloadBytes, _ := json.Marshal(struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
		ExitCode    int    `json:"exit_code"`
		Stdout      string `json:"stdout"`
		Stderr      string `json:"stderr"`
		Error       string `json:"error"`
		DurationMs  int64  `json:"duration_ms"`
	}{req.ExecutionID, req.TargetID, req.RunnerID, req.LeaseID, req.ExitCode, req.Stdout, req.Stderr, req.Error, req.DurationMs})
	hash := fmt.Sprintf("%x", sha256.Sum256(payloadBytes))

	tx, err := beginAPITx(ctx, db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start result transaction"})
		return
	}
	defer tx.Rollback()
	var receiptHash, receiptBody string
	var receiptCode int
	if err := apiQueryRow(ctx, tx, `SELECT payload_hash, response_code, response_body
		FROM execution_result_receipts WHERE target_id = ? AND lease_id = ?`, req.TargetID, req.LeaseID).
		Scan(&receiptHash, &receiptCode, &receiptBody); err == nil {
		if receiptHash != hash {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "result payload conflicts with existing receipt"})
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to replay result receipt"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(receiptCode)
		_, _ = w.Write([]byte(receiptBody))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect result receipt"})
		return
	}

	var attempt, maxAttempts int
	var currentStatus string
	if err := apiQueryRow(ctx, tx, `SELECT status, attempt, max_attempts FROM execution_targets
		WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running'`,
		req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID).Scan(&currentStatus, &attempt, &maxAttempts); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "target is not assigned to this runner"})
		return
	}
	stdoutArtifactID, stderrArtifactID, err := persistExecutionArtifacts(ctx, tx, orgID, req.ExecutionID, req.TargetID, req.Stdout, req.Stderr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist execution output"})
		return
	}
	storedStdout, storedStderr := req.Stdout, req.Stderr
	if stdoutArtifactID != "" {
		storedStdout = ""
	}
	if stderrArtifactID != "" {
		storedStderr = ""
	}
	if status == "failed" && attempt >= maxAttempts {
		status = "dead_letter"
	}

	targetResult, err := apiExec(ctx, tx, "UPDATE execution_targets SET status = ?, exit_code = ?, error_summary = ?, stdout = ?, stderr = ?, stdout_artifact_id = ?, stderr_artifact_id = ?, stdout_bytes = ?, stderr_bytes = ?, lease_expires_at = NULL, finished_at = ? WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running' AND EXISTS (SELECT 1 FROM executions e WHERE e.id = execution_targets.execution_id AND e.organisation_id = ?) AND EXISTS (SELECT 1 FROM runners rn WHERE rn.id = execution_targets.runner_id AND rn.organisation_id = ?)",
		status, req.ExitCode, sqlNullString(req.Error), storedStdout, storedStderr, sqlNullString(stdoutArtifactID), sqlNullString(stderrArtifactID), len(req.Stdout), len(req.Stderr), time.Now().UTC(), req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID, orgID, orgID)
	if err != nil || func() bool { n, _ := targetResult.RowsAffected(); return n == 0 }() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "target is not assigned to this runner"})
		return
	}
	if status == "dead_letter" {
		if _, err := apiExec(ctx, tx, "UPDATE execution_targets SET error_summary = COALESCE(NULLIF(error_summary,''), 'maximum attempts exhausted') WHERE id = ?", req.TargetID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to mark dead-letter job"})
			return
		}
	}
	if status == "failed" && attempt < maxAttempts {
		backoff := retryBackoffSeconds(attempt)
		retryResult, err := apiExec(ctx, tx, `UPDATE execution_targets
			SET status = 'pending', runner_id = NULL, lease_id = NULL,
			    lease_expires_at = NULL, finished_at = NULL,
			    next_attempt_at = ?
			WHERE id = ? AND status = 'failed'`, time.Now().UTC().Add(time.Duration(backoff)*time.Second), req.TargetID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to schedule retry"})
			return
		}
		if affected, _ := retryResult.RowsAffected(); affected != 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "target retry state changed"})
			return
		}
		if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, req.TargetID, "failed", "pending", "execution.target.retry_scheduled", map[string]any{
			"retry_at_seconds": backoff, "attempt": attempt, "max_attempts": maxAttempts,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record retry timeline"})
			return
		}
		if err := writeAuditEventTx(ctx, tx, orgID, "", "execution.retry_scheduled", "execution", req.ExecutionID, "pending", map[string]any{
			"target_id": req.TargetID, "attempt": attempt, "max_attempts": maxAttempts,
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record retry audit"})
			return
		}
		responseBody := `{"status":"retry_scheduled"}`
		if err := insertResultReceipt(ctx, tx, orgID, req, hash, responseBody); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record result receipt"})
			return
		}
		if err := tx.Commit(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit result"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "retry_scheduled"})
		return
	}
	var remaining, failed int
	if err := apiQueryRow(ctx, tx, "SELECT COUNT(*) FROM execution_targets WHERE execution_id = ? AND status IN ('pending','running')", req.ExecutionID).Scan(&remaining); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution state"})
		return
	}
	if err := apiQueryRow(ctx, tx, "SELECT COUNT(*) FROM execution_targets WHERE execution_id = ? AND status IN ('failed','dead_letter')", req.ExecutionID).Scan(&failed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to inspect execution result"})
		return
	}
	if remaining == 0 {
		finalStatus := "succeeded"
		if failed > 0 {
			finalStatus = "failed"
		}
		finalizeResult, err := apiExec(ctx, tx, "UPDATE executions SET status = ?, finished_at = ?, error_summary = ? WHERE id = ? AND organisation_id = ? AND status = 'running'", finalStatus, time.Now().UTC(), sqlNullString(req.Error), req.ExecutionID, orgID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to finalize execution"})
			return
		}
		if affected, _ := finalizeResult.RowsAffected(); affected != 1 {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "execution is no longer running"})
			return
		}
		status = finalStatus
	}
	if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, req.TargetID, currentStatus, status, "execution.target.completed", map[string]any{"exit_code": req.ExitCode, "duration_ms": req.DurationMs}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
		return
	}
	if remaining == 0 {
		if err := recordExecutionEvent(ctx, tx, orgID, req.ExecutionID, "", "running", status, "execution.completed", map[string]any{"failed_targets": failed}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution timeline"})
			return
		}
	}
	action := "execution.target.completed"
	if remaining == 0 {
		action = "execution.completed"
	}
	if err := writeAuditEventTx(ctx, tx, orgID, "", action, "execution", req.ExecutionID, status, map[string]any{
		"exit_code": req.ExitCode, "duration_ms": req.DurationMs,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record execution audit"})
		return
	}
	responseBody := `{"status":"ok"}`
	if err := insertResultReceipt(ctx, tx, orgID, req, hash, responseBody); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to record result receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit result"})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func handleRenewLease(ctx context.Context, db *sql.DB, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		ExecutionID string `json:"execution_id"`
		TargetID    string `json:"target_id"`
		RunnerID    string `json:"runner_id"`
		LeaseID     string `json:"lease_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExecutionID == "" || req.TargetID == "" || req.RunnerID == "" || req.LeaseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid lease renewal body"})
		return
	}
	orgID, err := authenticateRunnerForID(db, r, req.RunnerID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
		return
	}
	ctx, cleanup, err := bindTenantConnection(ctx, db, orgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tenant database context unavailable"})
		return
	}
	defer cleanup()
	r = r.WithContext(ctx)
	leaseExpires := time.Now().UTC().Add(5 * time.Minute)
	claimed, err := apiCheckedExec(ctx, db, `UPDATE execution_targets
		SET lease_expires_at = ?
		WHERE id = ? AND execution_id = ? AND runner_id = ? AND lease_id = ? AND status = 'running'`,
		leaseExpires, req.TargetID, req.ExecutionID, req.RunnerID, req.LeaseID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to renew lease"})
		return
	}
	if !claimed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "lease is no longer active"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renewed"})
}

func insertResultReceipt(ctx context.Context, tx *sql.Tx, orgID string, req struct {
	ExecutionID string `json:"execution_id"`
	TargetID    string `json:"target_id"`
	RunnerID    string `json:"runner_id"`
	LeaseID     string `json:"lease_id"`
	ExitCode    int    `json:"exit_code"`
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	Error       string `json:"error"`
	DurationMs  int64  `json:"duration_ms"`
}, payloadHash, responseBody string) error {
	_, err := apiExec(ctx, tx, `INSERT INTO execution_result_receipts
		(organisation_id, execution_id, target_id, lease_id, runner_id, payload_hash, response_code, response_body)
		VALUES (?, ?, ?, ?, ?, ?, 200, ?)`, orgID, req.ExecutionID, req.TargetID, req.LeaseID, req.RunnerID, payloadHash, responseBody)
	return err
}

const artifactInlineLimit = 64 * 1024

func persistExecutionArtifacts(ctx context.Context, db auditExec, orgID, executionID, targetID, stdout, stderr string) (string, string, error) {
	if apiArtifacts == nil {
		return "", "", nil
	}
	var stdoutID, stderrID string
	store := func(kind, value string) (string, error) {
		if len(value) <= artifactInlineLimit {
			return "", nil
		}
		id := fmt.Sprintf("art_%s_%s_%s", executionID, targetID, kind)
		meta, err := apiArtifacts.Put(id, "text/plain", []byte(value))
		if err != nil {
			return "", err
		}
		backend := apiBackends.ArtifactStore
		if backend == "" {
			backend = "local"
		}
		if _, err := apiExec(ctx, db, `INSERT INTO artifact_records (id, organisation_id, owner_type, owner_id, content_type, byte_size, sha256, backend)
			VALUES (?,?,?,?,?,?,?,?)
			ON CONFLICT (id) DO UPDATE SET organisation_id = excluded.organisation_id,
			owner_type = excluded.owner_type, owner_id = excluded.owner_id,
			content_type = excluded.content_type, byte_size = excluded.byte_size,
			sha256 = excluded.sha256, backend = excluded.backend`, id, orgID, "execution_target_"+kind, targetID, meta.ContentType, meta.Size, meta.SHA256, backend); err != nil {
			return "", err
		}
		return id, nil
	}
	var err error
	if stdoutID, err = store("stdout", stdout); err != nil {
		return "", "", err
	}
	if stderrID, err = store("stderr", stderr); err != nil {
		return "", "", err
	}
	return stdoutID, stderrID, nil
}
