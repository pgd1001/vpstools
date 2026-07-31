package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pgd1001/svrtools/packages/ai"
	"github.com/pgd1001/svrtools/packages/artifacts"
	"github.com/pgd1001/svrtools/packages/authz"
	"github.com/pgd1001/svrtools/packages/config"
	"github.com/pgd1001/svrtools/packages/dispatch"
	"github.com/pgd1001/svrtools/packages/jobsign"
	"github.com/pgd1001/svrtools/packages/redact"
)

var version = "0.1.0-beta.1"

type tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var policy = authz.NewPolicy()
var apiDB *sql.DB

// apiReadDB serves read-only queries. It is nil for backends that pool
// connections themselves, in which case reads use the primary handle.
var apiReadDB *sql.DB
var apiArtifacts artifacts.Store
var apiBackends config.BackendConfig
var apiDispatcher dispatch.Publisher

// apiJobSigner authenticates every job handed to a runner. The runner refuses
// jobs it cannot verify, so this is what keeps the control plane the only
// component that can decide what runs on a server.
var apiJobSigner *jobsign.Signer

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
	signer, err := jobsign.NewSigner(apiBackends.JobSigningKey)
	if err != nil {
		logger.Error("job signing key invalid", "error", err)
		os.Exit(1)
	}
	apiJobSigner = signer
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
	// SQLite serialises writers but not WAL readers, so read traffic gets its
	// own pool. Without this every list query queues behind the single writer
	// and a handful of polling runners saturate the whole API.
	readDB, err := openMetadataReadDatabase(ctx, apiBackends)
	if err != nil {
		logger.Error("read pool open failed", "error", err)
		os.Exit(1)
	}
	if readDB != nil {
		defer readDB.Close()
		apiReadDB = readDB
	}
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
	artifactMetricsDir := ""
	if apiBackends.ArtifactStore == "local" {
		artifactMetricsDir = apiBackends.ArtifactsDir
	}
	registerRoutes(mux, db, artifactMetricsDir)

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

func dbFrom(r *http.Request) *sql.DB {
	return r.Context().Value(dbKey).(*sql.DB)
}

func dbFromRequest(r *http.Request) *sql.DB {
	if v := r.Context().Value(dbKey); v != nil {
		return v.(*sql.DB)
	}
	return apiDB
}

// readDBFrom returns the handle a read-only handler should use.
//
// When a request is pinned to a tenant connection (PostgreSQL row-level
// security), that pinned connection carries the tenant GUC and must be used,
// so the read pool is bypassed. Otherwise SQLite reads go to the wider read
// pool instead of contending for the single writer.
func readDBFrom(r *http.Request) *sql.DB {
	if tenantConnection(r.Context()) != nil {
		return dbFrom(r)
	}
	if apiReadDB != nil {
		return apiReadDB
	}
	return dbFrom(r)
}

func sqlNullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}
