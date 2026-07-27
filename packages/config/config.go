package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/artifacts"
)

// BackendConfig describes the deployment tier selected by environment variables.
// The default tier intentionally needs no external services.
type BackendConfig struct {
	DatabaseDriver         string
	DatabaseURL            string
	ArtifactStore          string
	ArtifactsDir           string
	JobDispatch            string
	Scheduler              string
	EventBus               string
	ArtifactKey            string
	S3Endpoint             string
	S3Bucket               string
	S3Region               string
	S3Prefix               string
	S3AccessKeyID          string
	S3SecretAccessKey      string
	S3SessionToken         string
	S3EncryptionKey        string
	S3ServerSideEncryption string
	S3SSEKMSKeyID          string
	S3Timeout              time.Duration
	S3MaxRetries           int
	S3RetryBackoff         time.Duration
	ApprovalExpirySeconds  int
}

func Load() BackendConfig {
	databaseURL := envOrDefault("DATABASE_URL", envOrDefault("DB_PATH", "./svrtools.db"))
	artifactsDir := envOrDefault("ARTIFACTS_DIR", envOrDefault("VPS_ARTIFACTS_DIR", "./data/artifacts"))
	approvalExpiry := 3600
	if value := os.Getenv("APPROVAL_EXPIRY_SECONDS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			approvalExpiry = parsed
		} else {
			approvalExpiry = -1
		}
	}
	return BackendConfig{
		DatabaseDriver:         envOrDefault("DATABASE_DRIVER", "sqlite"),
		DatabaseURL:            databaseURL,
		ArtifactStore:          envOrDefault("ARTIFACT_STORE", "local"),
		ArtifactsDir:           filepath.Clean(artifactsDir),
		JobDispatch:            envOrDefault("JOB_DISPATCH", "database"),
		Scheduler:              envOrDefault("SCHEDULER", "embedded"),
		EventBus:               envOrDefault("EVENT_BUS", "disabled"),
		ArtifactKey:            os.Getenv("ARTIFACT_ENCRYPTION_KEY"),
		S3Endpoint:             os.Getenv("S3_ENDPOINT"),
		S3Bucket:               os.Getenv("S3_BUCKET"),
		S3Region:               os.Getenv("S3_REGION"),
		S3Prefix:               os.Getenv("S3_PREFIX"),
		S3AccessKeyID:          os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:      os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3SessionToken:         os.Getenv("S3_SESSION_TOKEN"),
		S3EncryptionKey:        os.Getenv("S3_ENCRYPTION_KEY"),
		S3ServerSideEncryption: os.Getenv("S3_SERVER_SIDE_ENCRYPTION"),
		S3SSEKMSKeyID:          os.Getenv("S3_SSE_KMS_KEY_ID"),
		S3Timeout:              durationEnv("S3_TIMEOUT"),
		S3MaxRetries:           intEnv("S3_MAX_RETRIES"),
		S3RetryBackoff:         durationEnv("S3_RETRY_BACKOFF"),
		ApprovalExpirySeconds:  approvalExpiry,
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
	if c.ArtifactStore == "s3" {
		if _, err := artifacts.NewS3Store(c.S3Config()); err != nil {
			return fmt.Errorf("invalid S3 configuration: %w", err)
		}
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
	if c.ApprovalExpirySeconds < 60 || c.ApprovalExpirySeconds > 30*24*60*60 {
		return fmt.Errorf("APPROVAL_EXPIRY_SECONDS must be between 60 and 2592000")
	}
	return nil
}

func (c BackendConfig) S3Config() artifacts.S3Config {
	return artifacts.S3Config{
		Endpoint: c.S3Endpoint, Bucket: c.S3Bucket, Region: c.S3Region, Prefix: c.S3Prefix,
		AccessKeyID: c.S3AccessKeyID, SecretAccessKey: c.S3SecretAccessKey, SessionToken: c.S3SessionToken,
		EncryptionKey: c.S3EncryptionKey, ServerSideEncryption: c.S3ServerSideEncryption,
		SSEKMSKeyID: c.S3SSEKMSKeyID, Timeout: c.S3Timeout, MaxRetries: c.S3MaxRetries,
		RetryBackoff: c.S3RetryBackoff,
	}
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

func durationEnv(key string) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}

func intEnv(key string) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}
