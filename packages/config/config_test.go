package config

import "testing"

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
