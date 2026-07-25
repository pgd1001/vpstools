package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// BackendConfig describes the deployment tier selected by environment variables.
// The default tier intentionally needs no external services.
type BackendConfig struct {
	DatabaseDriver string
	DatabaseURL    string
	ArtifactStore  string
	ArtifactsDir   string
	JobDispatch    string
	Scheduler      string
	EventBus       string
	ArtifactKey    string
}

func Load() BackendConfig {
	databaseURL := envOrDefault("DATABASE_URL", envOrDefault("DB_PATH", "./svrtools.db"))
	artifactsDir := envOrDefault("ARTIFACTS_DIR", envOrDefault("VPS_ARTIFACTS_DIR", "./data/artifacts"))
	return BackendConfig{
		DatabaseDriver: envOrDefault("DATABASE_DRIVER", "sqlite"),
		DatabaseURL:    databaseURL,
		ArtifactStore:  envOrDefault("ARTIFACT_STORE", "local"),
		ArtifactsDir:   filepath.Clean(artifactsDir),
		JobDispatch:    envOrDefault("JOB_DISPATCH", "database"),
		Scheduler:      envOrDefault("SCHEDULER", "embedded"),
		EventBus:       envOrDefault("EVENT_BUS", "disabled"),
		ArtifactKey:    os.Getenv("ARTIFACT_ENCRYPTION_KEY"),
	}
}

func (c BackendConfig) Validate() error {
	if c.DatabaseDriver != "sqlite" && c.DatabaseDriver != "postgres" {
		return fmt.Errorf("unsupported DATABASE_DRIVER %q, expected sqlite or postgres", c.DatabaseDriver)
	}
	if c.ArtifactStore != "local" && c.ArtifactStore != "s3" {
		return fmt.Errorf("unsupported ARTIFACT_STORE %q, expected local or s3", c.ArtifactStore)
	}
	if c.JobDispatch != "database" && c.JobDispatch != "jetstream" {
		return fmt.Errorf("unsupported JOB_DISPATCH %q, expected database or jetstream", c.JobDispatch)
	}
	if c.Scheduler != "embedded" && c.Scheduler != "external" {
		return fmt.Errorf("unsupported SCHEDULER %q, expected embedded or external", c.Scheduler)
	}
	if c.EventBus != "disabled" && c.EventBus != "nats" {
		return fmt.Errorf("unsupported EVENT_BUS %q, expected disabled or nats", c.EventBus)
	}
	if c.DatabaseDriver == "postgres" && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("DATABASE_URL is required when DATABASE_DRIVER=postgres")
	}
	if c.ArtifactStore == "s3" && os.Getenv("S3_ENDPOINT") == "" {
		return fmt.Errorf("S3_ENDPOINT is required when ARTIFACT_STORE=s3")
	}
	if c.JobDispatch == "jetstream" && os.Getenv("NATS_URL") == "" {
		return fmt.Errorf("NATS_URL is required when JOB_DISPATCH=jetstream")
	}
	if c.EventBus == "nats" && os.Getenv("NATS_URL") == "" {
		return fmt.Errorf("NATS_URL is required when EVENT_BUS=nats")
	}
	if c.ArtifactStore == "local" && c.ArtifactsDir == "." {
		return fmt.Errorf("ARTIFACTS_DIR must not be the current directory")
	}
	return nil
}

func (c BackendConfig) Tier() string {
	if c.DatabaseDriver == "sqlite" && c.ArtifactStore == "local" && c.JobDispatch == "database" && c.Scheduler == "embedded" && c.EventBus == "disabled" {
		return "self-contained"
	}
	return "extended"
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
