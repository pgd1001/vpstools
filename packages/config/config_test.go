package config

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultConfigurationIsSelfContained(t *testing.T) {
	setSigningKey(t)
	t.Setenv("DATABASE_DRIVER", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("ARTIFACT_STORE", "")
	t.Setenv("ARTIFACTS_DIR", "")
	t.Setenv("VPS_ARTIFACTS_DIR", "")
	t.Setenv("JOB_DISPATCH", "")
	t.Setenv("SCHEDULER", "")
	t.Setenv("EVENT_BUS", "")
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Tier() != "self-contained" || c.DatabaseDriver != "sqlite" || c.ArtifactStore != "local" {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestExtendedConfigurationRequiresConnectionSettings(t *testing.T) {
	setSigningKey(t)
	t.Setenv("DATABASE_DRIVER", "postgres")
	t.Setenv("DATABASE_URL", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected postgres configuration error")
	}
}

func TestPostgresRLSRequiresPostgresDriver(t *testing.T) {
	setSigningKey(t)
	t.Setenv("POSTGRES_RLS", "true")
	t.Setenv("DATABASE_DRIVER", "sqlite")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "POSTGRES_RLS") {
		t.Fatalf("expected RLS driver validation error, got %v", err)
	}
	t.Setenv("DATABASE_DRIVER", "postgres")
	t.Setenv("DATABASE_URL", "postgres://user:secret@localhost:5432/db?sslmode=disable")
	if err := Load().Validate(); err != nil {
		t.Fatalf("valid PostgreSQL RLS configuration rejected: %v", err)
	}
}

func TestS3ConfigurationRequiresCompleteStoreSettings(t *testing.T) {
	setSigningKey(t)
	server := httptest.NewServer(nil)
	defer server.Close()
	for key, value := range map[string]string{
		"ARTIFACT_STORE": "s3", "S3_ENDPOINT": server.URL, "S3_BUCKET": "artifacts",
		"S3_ACCESS_KEY_ID": "access", "S3_SECRET_ACCESS_KEY": "secret", "S3_MAX_RETRIES": "2",
		"S3_TIMEOUT": "2s", "S3_RETRY_BACKOFF": "50ms",
	} {
		t.Setenv(key, value)
	}
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatalf("complete S3 configuration rejected: %v", err)
	}
	if c.S3Config().Bucket != "artifacts" || c.S3Config().AccessKeyID != "access" {
		t.Fatalf("S3 settings were not loaded: %+v", c.S3Config())
	}
}

func TestMalformedS3ConfigurationFailsClearly(t *testing.T) {
	t.Setenv("ARTIFACT_STORE", "s3")
	t.Setenv("S3_ENDPOINT", "not-a-url")
	t.Setenv("S3_BUCKET", "")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "invalid S3 configuration") {
		t.Fatalf("expected clear S3 configuration error, got %v", err)
	}
}

func TestApprovalExpiryCanBeConfiguredWithinSafeBounds(t *testing.T) {
	setSigningKey(t)
	t.Setenv("APPROVAL_EXPIRY_SECONDS", "7200")
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.ApprovalExpirySeconds != 7200 {
		t.Fatalf("approval expiry = %d, want 7200", c.ApprovalExpirySeconds)
	}

	t.Setenv("APPROVAL_EXPIRY_SECONDS", "30")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected short approval expiry to be rejected")
	}
	t.Setenv("APPROVAL_EXPIRY_SECONDS", "not-a-number")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected non-numeric approval expiry to be rejected")
	}
}

func TestAIConfigurationRequiresProviderEndpointAndModel(t *testing.T) {
	setSigningKey(t)
	t.Setenv("AI_PROVIDER", "openai-compatible")
	t.Setenv("AI_ENDPOINT", "")
	t.Setenv("AI_MODEL", "")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "AI_ENDPOINT") {
		t.Fatalf("expected missing endpoint error, got %v", err)
	}
	t.Setenv("AI_ENDPOINT", "http://localhost:11434/v1")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "AI_MODEL") {
		t.Fatalf("expected missing model error, got %v", err)
	}
	t.Setenv("AI_MODEL", "local-model")
	if err := Load().Validate(); err != nil {
		t.Fatalf("valid AI configuration rejected: %v", err)
	}
	t.Setenv("AI_ENDPOINT", "file:///tmp/model")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "AI_ENDPOINT") {
		t.Fatalf("expected invalid endpoint error, got %v", err)
	}
}

func TestJetStreamDispatchLoadsAndValidatesBoundedConsumerSettings(t *testing.T) {
	setSigningKey(t)
	t.Setenv("JOB_DISPATCH", "jetstream")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("NATS_STREAM", "SVRTOOLS_JOBS")
	t.Setenv("NATS_SUBJECT", "svrtools.jobs.available")
	t.Setenv("NATS_DURABLE", "runner-fleet")
	t.Setenv("NATS_MAX_DELIVER", "7")
	t.Setenv("NATS_ACK_WAIT", "20s")
	t.Setenv("NATS_DUPLICATE_WINDOW", "3m")
	c := Load()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.NATSMaxDeliver != 7 || c.NATSAckWait != 20*time.Second || c.NATSDuplicateWindow != 3*time.Minute {
		t.Fatalf("unexpected JetStream settings: %+v", c)
	}

	t.Setenv("NATS_MAX_DELIVER", "21")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected an unsafe redelivery bound to fail")
	}
}

// setSigningKey supplies the mandatory job signing key so each test exercises
// the setting it actually cares about rather than the signing-key gate.
func setSigningKey(t *testing.T) {
	t.Helper()
	t.Setenv("JOB_SIGNING_KEY", "config-test-signing-key-at-least-32ch")
}

// Job signing is the runner's only defence against an unauthorised command, so
// a missing or weak key must fail configuration validation in every tier
// rather than degrading to unsigned dispatch.
func TestJobSigningKeyIsRequiredAndMustBeStrong(t *testing.T) {
	t.Setenv("JOB_SIGNING_KEY", "")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "JOB_SIGNING_KEY") {
		t.Fatalf("missing signing key error = %v, want a JOB_SIGNING_KEY failure", err)
	}
	t.Setenv("JOB_SIGNING_KEY", "too-short")
	if err := Load().Validate(); err == nil || !strings.Contains(err.Error(), "JOB_SIGNING_KEY") {
		t.Fatalf("short signing key error = %v, want a JOB_SIGNING_KEY failure", err)
	}
	setSigningKey(t)
	if err := Load().Validate(); err != nil {
		t.Fatalf("valid signing key rejected: %v", err)
	}
}

// An environment that is not explicitly non-production must be treated as
// production, so a forgotten or misspelled variable cannot enable development
// conveniences in a real deployment.
func TestProductionModeFailsClosed(t *testing.T) {
	for _, value := range []string{"", "production", "prod", "prod-eu", "staging", "typo"} {
		t.Setenv("VPS_ENV", value)
		t.Setenv("APP_ENV", "")
		t.Setenv("ENVIRONMENT", "")
		if !ProductionMode() {
			t.Fatalf("VPS_ENV=%q was treated as non-production", value)
		}
	}
	for _, value := range []string{"dev", "development", "local", "test", "ci"} {
		t.Setenv("VPS_ENV", value)
		if ProductionMode() {
			t.Fatalf("VPS_ENV=%q was treated as production", value)
		}
	}
}
