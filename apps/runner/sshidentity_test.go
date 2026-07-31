package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgd1001/svrtools/packages/sshcreds"
)

// The runner must refuse to connect to a server the control plane has not
// pinned a host key for. Before per-server SSH identity a single shared
// known_hosts file governed the whole fleet, so a host absent from it was
// merely an operator inconvenience. Now the refusal is explicit and is
// reported back as a target failure the control plane can see.
func TestRunTargetRefusesServerWithoutPinnedHostKey(t *testing.T) {
	keystore := sshcreds.NewKeystore(t.TempDir())
	j := &job{Command: "uptime", CredentialRef: "web-prod"}

	result := runTarget(context.Background(), keystore, j, "host.example.com", 22, "operator")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "no ssh host key fingerprint") {
		t.Fatalf("expected a host key refusal, got %q", result.Error)
	}
}

// A server with no recorded credential reference must fail rather than fall
// back to a shared identity.
func TestRunTargetRefusesServerWithoutCredentialReference(t *testing.T) {
	keystore := sshcreds.NewKeystore(t.TempDir())
	j := &job{Command: "uptime", HostKeyFingerprint: "SHA256:" + strings.Repeat("A", 43)}

	result := runTarget(context.Background(), keystore, j, "host.example.com", 22, "operator")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "no ssh credential reference") {
		t.Fatalf("expected a credential refusal, got %q", result.Error)
	}
}

// A credential this runner does not hold must fail closed rather than
// substituting another key that happens to be present in the keystore.
func TestRunTargetRefusesUnresolvableCredential(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other-server"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	keystore := sshcreds.NewKeystore(dir)
	j := &job{
		Command:            "uptime",
		CredentialRef:      "web-prod",
		HostKeyFingerprint: "SHA256:" + strings.Repeat("A", 43),
	}

	result := runTarget(context.Background(), keystore, j, "host.example.com", 22, "operator")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "credential unavailable") {
		t.Fatalf("expected a credential resolution failure, got %q", result.Error)
	}
}
