package runbooks_test

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/pgd1001/svrtools/packages/runbooks"
)

func loadRunbookTestData(t *testing.T, name string) string {
	t.Helper()
	clean := strings.Map(func(r rune) rune {
		if r == '_' {
			return '-'
		}
		return r
	}, name)
	if strings.HasSuffix(clean, ".yml") {
		clean = clean[:len(clean)-4]
	}
	clean = strings.TrimPrefix(clean, "validate_")

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	var candidates []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".yml")
		if base == clean {
			candidates = append(candidates, e.Name())
		}
	}

	if len(candidates) == 0 {
		t.Fatalf("no YAML file found matching %q (tried %q)", name, clean)
	}
	b, err := os.ReadFile(candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseAllRunbookTemplates(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("base-hardened-ubuntu", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "base-hardened-ubuntu")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "base-hardened-ubuntu" {
			t.Errorf("expected name base-hardened-ubuntu, got %s", rb.Metadata.Name)
		}
		if rb.Spec.Execution.Command == "" {
			t.Error("command is empty")
		}
	})

	t.Run("docker-server", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "docker-server")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "docker-server" {
			t.Errorf("expected name docker-server, got %s", rb.Metadata.Name)
		}
		if rb.Spec.Execution.Command == "" {
			t.Error("command is empty")
		}
	})

	t.Run("dokploy-install", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "dokploy-install")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "dokploy-install" {
			t.Errorf("expected name dokploy-install, got %s", rb.Metadata.Name)
		}
	})

	t.Run("nextcloud-aio", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "nextcloud-aio")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "nextcloud-aio" {
			t.Errorf("expected name nextcloud-aio, got %s", rb.Metadata.Name)
		}
	})

	t.Run("seafile-install", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "seafile-install")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "seafile-install" {
			t.Errorf("expected name seafile-install, got %s", rb.Metadata.Name)
		}
	})

	t.Run("hermes-agent", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "hermes-agent")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "hermes-agent" {
			t.Errorf("expected name hermes-agent, got %s", rb.Metadata.Name)
		}
	})

	t.Run("ai-code-tools", func(t *testing.T) {
		yaml := loadRunbookTestData(t, "ai-code-tools")
		rb, err := runbooks.Parse(yaml)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if rb.Metadata.Name != "ai-code-tools" {
			t.Errorf("expected name ai-code-tools, got %s", rb.Metadata.Name)
		}
	})

	// Verify all .yml files are validated
	validated := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") || strings.HasSuffix(e.Name(), "_test.yml") {
			continue
		}
		t.Run("all-"+e.Name(), func(t *testing.T) {
			b, err := os.ReadFile(e.Name())
			if err != nil {
				t.Fatal(err)
			}
			rb, err := runbooks.Parse(string(b))
			if err != nil {
				t.Fatalf("validate %s: %v", e.Name(), err)
			}
			if unicode.IsUpper(rune(rb.Metadata.Name[0])) {
				t.Errorf("name %q should be lowercase", rb.Metadata.Name)
			}
		})
		validated++
	}
	t.Logf("validated %d runbook files", validated)
}
