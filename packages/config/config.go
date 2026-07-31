package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pgd1001/svrtools/packages/artifacts"
	"github.com/pgd1001/svrtools/packages/jobsign"
)

// BackendConfig describes the deployment tier selected by environment variables.
// The default tier intentionally needs no external services.
type BackendConfig struct {
	DatabaseDriver         string
	DatabaseURL            string
	PostgresRLS            bool
	ArtifactStore          string
	ArtifactsDir           string
	JobDispatch            string
	Scheduler              string
	EventBus               string
	NATSURL                string
	NATSStream             string
	NATSSubject            string
	NATSDurable            string
	NATSMaxDeliver         int
	NATSAckWait            time.Duration
	NATSDuplicateWindow    time.Duration
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
	AIProvider             string
	AIEndpoint             string
	AIAPIKey               string
	AIModel                string
	AITimeout              time.Duration
	AIMaxPromptBytes       int
	AIMaxResponseBytes     int64
	// JobSigningKey authenticates dispatched jobs end to end. The runner
	// refuses any job it cannot verify, so this is the key that keeps the
	// runner from executing commands the control plane did not authorise.
	JobSigningKey string
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
		PostgresRLS:            strings.EqualFold(strings.TrimSpace(os.Getenv("POSTGRES_RLS")), "true"),
		ArtifactStore:          envOrDefault("ARTIFACT_STORE", "local"),
		ArtifactsDir:           filepath.Clean(artifactsDir),
		JobDispatch:            envOrDefault("JOB_DISPATCH", "database"),
		Scheduler:              envOrDefault("SCHEDULER", "embedded"),
		EventBus:               envOrDefault("EVENT_BUS", "disabled"),
		NATSURL:                strings.TrimSpace(os.Getenv("NATS_URL")),
		NATSStream:             envOrDefault("NATS_STREAM", "SVRTOOLS_JOBS"),
		NATSSubject:            envOrDefault("NATS_SUBJECT", "svrtools.jobs.available"),
		NATSDurable:            envOrDefault("NATS_DURABLE", "svrtools-runners"),
		NATSMaxDeliver:         intEnvDefault("NATS_MAX_DELIVER", 5),
		NATSAckWait:            durationEnvDefault("NATS_ACK_WAIT", 30*time.Second),
		NATSDuplicateWindow:    durationEnvDefault("NATS_DUPLICATE_WINDOW", 2*time.Minute),
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
		AIProvider:             strings.TrimSpace(os.Getenv("AI_PROVIDER")),
		AIEndpoint:             strings.TrimSpace(os.Getenv("AI_ENDPOINT")),
		AIAPIKey:               os.Getenv("AI_API_KEY"),
		AIModel:                strings.TrimSpace(os.Getenv("AI_MODEL")),
		AITimeout:              durationEnv("AI_TIMEOUT"),
		AIMaxPromptBytes:       intEnv("AI_MAX_PROMPT_BYTES"),
		AIMaxResponseBytes:     int64(intEnv("AI_MAX_RESPONSE_BYTES")),
		JobSigningKey:          strings.TrimSpace(os.Getenv("JOB_SIGNING_KEY")),
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
	if c.PostgresRLS && c.DatabaseDriver != "postgres" {
		return fmt.Errorf("POSTGRES_RLS=true requires DATABASE_DRIVER=postgres")
	}
	if c.ArtifactStore == "s3" {
		if _, err := artifacts.NewS3Store(c.S3Config()); err != nil {
			return fmt.Errorf("invalid S3 configuration: %w", err)
		}
	}
	if c.JobDispatch == "jetstream" && c.NATSURL == "" {
		return fmt.Errorf("NATS_URL is required when JOB_DISPATCH=jetstream")
	}
	if c.JobDispatch == "jetstream" {
		natsEndpoint, err := url.Parse(c.NATSURL)
		if err != nil || natsEndpoint.Host == "" || (natsEndpoint.Scheme != "nats" && natsEndpoint.Scheme != "tls" && natsEndpoint.Scheme != "ws" && natsEndpoint.Scheme != "wss") {
			return fmt.Errorf("NATS_URL must be an absolute nats, tls, ws, or wss URL")
		}
	}
	if c.EventBus == "nats" && c.NATSURL == "" {
		return fmt.Errorf("NATS_URL is required when EVENT_BUS=nats")
	}
	if c.JobDispatch == "jetstream" {
		if c.NATSStream == "" || c.NATSSubject == "" || c.NATSDurable == "" {
			return fmt.Errorf("NATS_STREAM, NATS_SUBJECT, and NATS_DURABLE are required when JOB_DISPATCH=jetstream")
		}
		if c.NATSMaxDeliver < 1 || c.NATSMaxDeliver > 20 {
			return fmt.Errorf("NATS_MAX_DELIVER must be between 1 and 20")
		}
		if c.NATSAckWait <= 0 {
			return fmt.Errorf("NATS_ACK_WAIT must be positive")
		}
		if c.NATSDuplicateWindow <= 0 {
			return fmt.Errorf("NATS_DUPLICATE_WINDOW must be positive")
		}
	}
	if c.ArtifactStore == "local" && c.ArtifactsDir == "." {
		return fmt.Errorf("ARTIFACTS_DIR must not be the current directory")
	}
	if c.ApprovalExpirySeconds < 60 || c.ApprovalExpirySeconds > 30*24*60*60 {
		return fmt.Errorf("APPROVAL_EXPIRY_SECONDS must be between 60 and 2592000")
	}
	if c.AIProvider != "" && c.AIProvider != "openai-compatible" {
		return fmt.Errorf("unsupported AI_PROVIDER %q, expected openai-compatible", c.AIProvider)
	}
	if c.AIProvider != "" && c.AIEndpoint == "" {
		return fmt.Errorf("AI_ENDPOINT is required when AI_PROVIDER is configured")
	}
	if c.AIProvider != "" && c.AIModel == "" {
		return fmt.Errorf("AI_MODEL is required when AI_PROVIDER is configured")
	}
	if c.AIProvider != "" {
		endpoint, err := url.Parse(c.AIEndpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return fmt.Errorf("AI_ENDPOINT must be an absolute http or https URL")
		}
	}
	if c.AITimeout < 0 || c.AIMaxPromptBytes < 0 || c.AIMaxResponseBytes < 0 {
		return fmt.Errorf("AI limits must not be negative")
	}
	// Job signing is not optional. The runner is not allowed to make access
	// decisions, so its only defence against an unauthorised job is a
	// signature it can verify. Allowing an unsigned mode "just for local
	// development" would mean the security property is never actually
	// exercised, so both tiers require a key.
	if c.JobSigningKey == "" {
		return fmt.Errorf("JOB_SIGNING_KEY is required so runners can verify that the control plane authorised each job")
	}
	if len(c.JobSigningKey) < jobsign.MinKeyLength {
		return fmt.Errorf("JOB_SIGNING_KEY must be at least %d characters", jobsign.MinKeyLength)
	}
	return nil
}

// ProductionMode reports whether the process is configured as a production
// deployment. It is deliberately conservative: anything that is not explicitly
// a non-production environment name counts as production, so an unset or
// misspelled environment fails closed rather than silently enabling
// development conveniences.
func ProductionMode() bool {
	for _, key := range []string{"VPS_ENV", "APP_ENV", "ENVIRONMENT"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value == "" {
			continue
		}
		switch value {
		case "dev", "development", "local", "test", "testing", "ci":
			return false
		default:
			// staging, prod, prod-eu, production, or anything unrecognised.
			return true
		}
	}
	// No environment declared at all. Treat as production so a forgotten
	// variable cannot open a development bypass.
	return true
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

func (c BackendConfig) DispatchConfig() (string, string, string, string, int, time.Duration, time.Duration) {
	return c.NATSURL, c.NATSStream, c.NATSSubject, c.NATSDurable, c.NATSMaxDeliver, c.NATSAckWait, c.NATSDuplicateWindow
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

func intEnvDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return -1
	}
	return parsed
}

func durationEnvDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}
