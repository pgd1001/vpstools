package config

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultConfigurationIsSelfContained(t *testing.T) {
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
	t.Setenv("DATABASE_DRIVER", "postgres")
	t.Setenv("DATABASE_URL", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("expected postgres configuration error")
	}
}

func TestS3ConfigurationRequiresCompleteStoreSettings(t *testing.T) {
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
