package sshcreds

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestResolveReturnsPrivateKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "web-prod"), "PRIVATE KEY MATERIAL")

	credential, err := NewKeystore(dir).Resolve("web-prod")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if string(credential.PrivateKey) != "PRIVATE KEY MATERIAL" {
		t.Fatalf("unexpected key material %q", credential.PrivateKey)
	}
}

// Each reference must resolve to its own material. This is the property that
// makes a single compromised host credential stop at that host.
func TestResolveIsPerReference(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "web-prod"), "web key")
	writeFile(t, filepath.Join(dir, "db-prod"), "db key")

	keystore := NewKeystore(dir)
	web, err := keystore.Resolve("web-prod")
	if err != nil {
		t.Fatalf("resolve web: %v", err)
	}
	database, err := keystore.Resolve("db-prod")
	if err != nil {
		t.Fatalf("resolve db: %v", err)
	}
	if string(web.PrivateKey) == string(database.PrivateKey) {
		t.Fatal("two references resolved to the same credential")
	}
}

// A reference the runner has no material for must fail rather than fall back
// to some other credential, which would execute as an identity the control
// plane did not record.
func TestResolveUnknownReferenceFails(t *testing.T) {
	if _, err := NewKeystore(t.TempDir()).Resolve("absent"); !errors.Is(err, ErrUnknownReference) {
		t.Fatalf("expected ErrUnknownReference, got %v", err)
	}
}

// The reference arrives from the API, so it must never be able to name a file
// outside the keystore directory.
func TestResolveRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "legitimate"), "key")

	for _, reference := range []string{
		"../secret",
		"..",
		".",
		"nested/key",
		`nested\key`,
		"/etc/shadow",
		".hidden",
		"",
	} {
		_, err := NewKeystore(dir).Resolve(reference)
		if err == nil {
			t.Fatalf("reference %q was accepted", reference)
		}
		if !errors.Is(err, ErrInvalidReference) && !errors.Is(err, ErrUnknownReference) {
			t.Fatalf("reference %q produced unexpected error %v", reference, err)
		}
	}
}

func TestResolveReadsPassphraseAndPassword(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "web-prod"), "key material")
	// The trailing newline is what an editor or `echo` leaves behind and must
	// not become part of the secret.
	writeFile(t, filepath.Join(dir, "web-prod.passphrase"), "unlock-me\n")

	credential, err := NewKeystore(dir).Resolve("web-prod")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if string(credential.Passphrase) != "unlock-me" {
		t.Fatalf("passphrase %q retained its newline or was not read", credential.Passphrase)
	}

	writeFile(t, filepath.Join(dir, "legacy.password"), "secret\n")
	legacy, err := NewKeystore(dir).Resolve("legacy")
	if err != nil {
		t.Fatalf("resolve legacy: %v", err)
	}
	if legacy.Password != "secret" {
		t.Fatalf("password %q was not read correctly", legacy.Password)
	}
}

// An unconfigured keystore must resolve nothing. The runner uses this to start
// in simulate mode without credentials while still failing closed on real work.
func TestUnconfiguredKeystoreResolvesNothing(t *testing.T) {
	keystore := NewKeystore("")
	if keystore.Configured() {
		t.Fatal("an empty directory should not report as configured")
	}
	if _, err := keystore.Resolve("anything"); !errors.Is(err, ErrUnknownReference) {
		t.Fatalf("expected ErrUnknownReference, got %v", err)
	}
}
