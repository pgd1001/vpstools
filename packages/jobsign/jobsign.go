// Package jobsign implements the integrity boundary between the control plane
// and the runners. The control plane is the only component allowed to decide
// what a runner executes, so every dispatched job carries a message
// authentication code over the fields that determine execution. A runner that
// cannot verify the code refuses to execute, which keeps a compromised or
// spoofed dispatch path from turning into remote command execution.
package jobsign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Version prefixes every signature so the scheme can be rotated without
// silently accepting an older, weaker construction.
const Version = "v1"

var (
	// ErrNoKey reports that signing or verification was attempted without a
	// configured key.
	ErrNoKey = errors.New("job signing key is not configured")
	// ErrMalformed reports a signature that is not in the expected format.
	ErrMalformed = errors.New("job signature is malformed")
	// ErrMismatch reports a signature that does not authenticate the job.
	ErrMismatch = errors.New("job signature does not match the job")
	// ErrExpired reports a signature presented after its validity window.
	ErrExpired = errors.New("job signature has expired")
)

// MinKeyLength is the shortest key accepted. A short shared secret would make
// the authentication code guessable, so it is rejected at configuration time
// rather than at dispatch time.
const MinKeyLength = 32

// Claims are the execution-determining fields covered by the signature. Any
// field that changes what a runner does must be listed here; a field outside
// the struct is not authenticated and must not influence execution.
type Claims struct {
	ExecutionID string `json:"execution_id"`
	TargetID    string `json:"target_id"`
	LeaseID     string `json:"lease_id"`
	RunnerID    string `json:"runner_id"`
	Command     string `json:"command"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Timeout     int    `json:"timeout"`
	// ExpiresAtUnix bounds replay. It is normally the lease expiry, so a
	// captured job cannot be replayed after the control plane has reassigned
	// the work.
	ExpiresAtUnix int64 `json:"expires_at_unix"`
}

// Signer produces signatures for dispatched jobs.
type Signer struct {
	key []byte
}

// NewSigner validates the key and returns a signer. An empty key returns
// ErrNoKey so callers can decide whether unsigned dispatch is acceptable for
// their deployment tier rather than defaulting to it silently.
func NewSigner(key string) (*Signer, error) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return nil, ErrNoKey
	}
	if len(trimmed) < MinKeyLength {
		return nil, fmt.Errorf("job signing key must be at least %d characters", MinKeyLength)
	}
	return &Signer{key: []byte(trimmed)}, nil
}

// Sign returns the signature for claims. The returned value is safe to place
// in a JSON response body.
func (s *Signer) Sign(claims Claims) (string, error) {
	if s == nil || len(s.key) == 0 {
		return "", ErrNoKey
	}
	payload, err := canonical(claims)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	return Version + ":" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Verify reports whether signature authenticates claims and is still inside
// its validity window. now is passed explicitly so expiry is testable and so
// the caller controls the clock source.
func (s *Signer) Verify(claims Claims, signature string, now time.Time) error {
	if s == nil || len(s.key) == 0 {
		return ErrNoKey
	}
	version, encoded, found := strings.Cut(strings.TrimSpace(signature), ":")
	if !found || version != Version || encoded == "" {
		return ErrMalformed
	}
	provided, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ErrMalformed
	}
	payload, err := canonical(claims)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(payload)
	// Compare before checking expiry so an attacker cannot use the error to
	// distinguish a valid-but-expired signature from an invalid one.
	if !hmac.Equal(mac.Sum(nil), provided) {
		return ErrMismatch
	}
	if claims.ExpiresAtUnix != 0 && now.UTC().Unix() > claims.ExpiresAtUnix {
		return ErrExpired
	}
	return nil
}

// canonical serialises claims deterministically. encoding/json emits struct
// fields in declaration order, so the same claims always produce the same
// bytes on both the signing and verifying side.
func canonical(claims Claims) ([]byte, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("serialise job claims: %w", err)
	}
	return append([]byte(Version+"\n"), payload...), nil
}
