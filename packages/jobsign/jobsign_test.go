package jobsign

import (
	"errors"
	"testing"
	"time"
)

const testKey = "test-signing-key-that-is-long-enough-32"

func sampleClaims() Claims {
	return Claims{
		ExecutionID:   "exe_1",
		TargetID:      "ext_1",
		LeaseID:       "lease_1",
		RunnerID:      "rnr_1",
		Command:       "uptime",
		Host:          "app.example.com",
		Port:          22,
		User:          "svrtools",
		Timeout:       300,
		ExpiresAtUnix: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Unix(),
	}
}

func mustSigner(t *testing.T, key string) *Signer {
	t.Helper()
	signer, err := NewSigner(key)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	return signer
}

func TestSignedJobVerifies(t *testing.T) {
	signer := mustSigner(t, testKey)
	claims := sampleClaims()
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := signer.Verify(claims, signature, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Verify on untampered job: %v", err)
	}
}

// A runner must refuse a job whose command was altered in transit. This is the
// property that stops a spoofed dispatch from becoming remote code execution.
func TestTamperedCommandIsRejected(t *testing.T) {
	signer := mustSigner(t, testKey)
	claims := sampleClaims()
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	tampered := claims
	tampered.Command = "rm -rf /"
	if err := signer.Verify(tampered, signature, time.Now()); !errors.Is(err, ErrMismatch) {
		t.Fatalf("tampered command error = %v, want ErrMismatch", err)
	}
}

// Every execution-determining field must be covered, not just the command. A
// swapped host would otherwise run the approved command on an unapproved
// server.
func TestEveryExecutionFieldIsCovered(t *testing.T) {
	signer := mustSigner(t, testKey)
	base := sampleClaims()
	signature, err := signer.Sign(base)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	mutations := map[string]func(*Claims){
		"execution_id": func(c *Claims) { c.ExecutionID = "exe_other" },
		"target_id":    func(c *Claims) { c.TargetID = "ext_other" },
		"lease_id":     func(c *Claims) { c.LeaseID = "lease_other" },
		"runner_id":    func(c *Claims) { c.RunnerID = "rnr_other" },
		"command":      func(c *Claims) { c.Command = "shutdown now" },
		"host":         func(c *Claims) { c.Host = "prod.example.com" },
		"port":         func(c *Claims) { c.Port = 2222 },
		"user":         func(c *Claims) { c.User = "root" },
		"timeout":      func(c *Claims) { c.Timeout = 9999 },
		"expires_at":   func(c *Claims) { c.ExpiresAtUnix = base.ExpiresAtUnix + 3600 },
	}
	for field, mutate := range mutations {
		t.Run(field, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			if err := signer.Verify(mutated, signature, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrMismatch) {
				t.Fatalf("mutating %s gave error %v, want ErrMismatch", field, err)
			}
		})
	}
}

func TestSignatureFromAnotherKeyIsRejected(t *testing.T) {
	claims := sampleClaims()
	signature, err := mustSigner(t, "an-entirely-different-key-also-32-chars").Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := mustSigner(t, testKey).Verify(claims, signature, time.Now()); !errors.Is(err, ErrMismatch) {
		t.Fatalf("foreign key error = %v, want ErrMismatch", err)
	}
}

// A captured job must not be replayable after its lease window closes.
func TestExpiredSignatureIsRejected(t *testing.T) {
	signer := mustSigner(t, testKey)
	claims := sampleClaims()
	claims.ExpiresAtUnix = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	signature, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := signer.Verify(claims, signature, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired signature error = %v, want ErrExpired", err)
	}
}

func TestMalformedSignaturesAreRejected(t *testing.T) {
	signer := mustSigner(t, testKey)
	claims := sampleClaims()
	for _, signature := range []string{"", "not-a-signature", "v2:abcdef", "v1:", "v1:!!!not-base64!!!"} {
		if err := signer.Verify(claims, signature, time.Now()); err == nil {
			t.Fatalf("signature %q was accepted", signature)
		}
	}
}

func TestShortAndEmptyKeysAreRefused(t *testing.T) {
	if _, err := NewSigner(""); !errors.Is(err, ErrNoKey) {
		t.Fatalf("empty key error = %v, want ErrNoKey", err)
	}
	if _, err := NewSigner("too-short"); err == nil {
		t.Fatal("a key shorter than MinKeyLength was accepted")
	}
}

// An unconfigured signer must fail closed rather than produce or accept
// anything.
func TestNilSignerFailsClosed(t *testing.T) {
	var signer *Signer
	if _, err := signer.Sign(sampleClaims()); !errors.Is(err, ErrNoKey) {
		t.Fatalf("nil signer Sign error = %v, want ErrNoKey", err)
	}
	if err := signer.Verify(sampleClaims(), "v1:abc", time.Now()); !errors.Is(err, ErrNoKey) {
		t.Fatalf("nil signer Verify error = %v, want ErrNoKey", err)
	}
}

func TestSigningIsDeterministic(t *testing.T) {
	signer := mustSigner(t, testKey)
	first, err := signer.Sign(sampleClaims())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	second, err := signer.Sign(sampleClaims())
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if first != second {
		t.Fatalf("signatures differ across calls: %q vs %q", first, second)
	}
}
