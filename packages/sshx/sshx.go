// Package sshx executes a single non-interactive command on a remote host.
//
// Two properties matter more than convenience here, because this package is
// the only place the product touches a customer's machines:
//
//   - The remote host proves its identity before a command is sent. The
//     control plane records a host key fingerprint when a server is
//     registered, and the runner refuses to continue when the key presented
//     at connection time does not match. Without that check, anything able to
//     answer on the host and port would receive commands intended for the
//     real server.
//   - Credentials are per-server, not per-fleet. The executor is constructed
//     with the material for one target, so compromising one host's credential
//     does not hand over the rest of the inventory.
package sshx

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ErrHostKeyMismatch reports that the host presented a key other than the one
// pinned for it. It is deliberately distinct from a generic dial failure: a
// mismatch may mean interception rather than an outage, so an operator needs
// to be able to tell the two apart.
var ErrHostKeyMismatch = errors.New("ssh host key does not match the pinned fingerprint")

// ErrNoHostKeyPinned reports that no fingerprint was supplied. Connecting
// without one would mean trusting whatever answers, so it is refused rather
// than downgraded.
var ErrNoHostKeyPinned = errors.New("no ssh host key fingerprint is pinned for this server")

// ErrNoCredential reports that no usable authentication material was supplied.
var ErrNoCredential = errors.New("no ssh credential was supplied")

// Credential is the authentication material for one target. Exactly one of
// PrivateKey or Password should be set; PrivateKey is preferred and Password
// exists only for hosts that cannot yet accept a key.
type Credential struct {
	// PrivateKey is a PEM-encoded private key.
	PrivateKey []byte
	// Passphrase decrypts PrivateKey when it is encrypted.
	Passphrase []byte
	// Password authenticates with a password instead of a key.
	Password string
}

// authMethods converts the credential into the ssh auth methods it supports.
func (c Credential) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if len(c.PrivateKey) > 0 {
		var signer ssh.Signer
		var err error
		if len(c.Passphrase) > 0 {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(c.PrivateKey, c.Passphrase)
		} else {
			signer, err = ssh.ParsePrivateKey(c.PrivateKey)
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if c.Password != "" {
		methods = append(methods, ssh.Password(c.Password))
	}
	if len(methods) == 0 {
		return nil, ErrNoCredential
	}
	return methods, nil
}

// FingerprintSHA256 renders a host key in the same form OpenSSH prints, so a
// fingerprint recorded from `ssh-keyscan` output can be compared directly with
// one produced here.
func FingerprintSHA256(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// pinnedHostKey returns a callback that accepts only the given fingerprint.
//
// The comparison is on the fingerprint rather than the key blob so the control
// plane can store a short, printable value that an operator can eyeball
// against what the host reports.
func pinnedHostKey(fingerprint string) (ssh.HostKeyCallback, error) {
	want := strings.TrimSpace(fingerprint)
	if want == "" {
		return nil, ErrNoHostKeyPinned
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		got := FingerprintSHA256(key)
		if got != want {
			return fmt.Errorf("%w: host %s presented %s, expected %s", ErrHostKeyMismatch, hostname, got, want)
		}
		return nil
	}, nil
}

// Executor runs commands against one target with one credential.
type Executor struct {
	addr            string
	user            string
	credential      Credential
	hostKeyCallback ssh.HostKeyCallback
}

// NewExecutor builds an executor for a single target.
//
// hostKeyFingerprint is required. There is no variant that skips the check,
// because a caller reaching for one would be turning off the only protection
// against sending a privileged command to an impostor.
func NewExecutor(addr, user string, credential Credential, hostKeyFingerprint string) (*Executor, error) {
	callback, err := pinnedHostKey(hostKeyFingerprint)
	if err != nil {
		return nil, err
	}
	if _, err := credential.authMethods(); err != nil {
		return nil, err
	}
	return &Executor{addr: addr, user: user, credential: credential, hostKeyCallback: callback}, nil
}

// Result is the outcome of one command.
type Result struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	Error      string
	DurationMs int64
}

// outputLimit bounds how much of each stream is read into memory. Output past
// this point is discarded rather than allowed to exhaust the runner.
const outputLimit = 2 << 20

// Run executes command and returns its output. A failure to reach or
// authenticate to the host is reported in Result.Error with exit code -1
// rather than as a Go error, so that every attempt produces a record the
// control plane can store against the target.
func (e *Executor) Run(ctx context.Context, command string) Result {
	start := time.Now()
	fail := func(format string, args ...any) Result {
		return Result{
			Error:      fmt.Sprintf(format, args...),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	methods, err := e.credential.authMethods()
	if err != nil {
		return fail("ssh credential unusable: %v", err)
	}

	config := &ssh.ClientConfig{
		User:            e.user,
		Auth:            methods,
		HostKeyCallback: e.hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", e.addr, config)
	if err != nil {
		if errors.Is(err, ErrHostKeyMismatch) {
			return fail("ssh host key verification failed: %v", err)
		}
		return fail("ssh dial failed: %v", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fail("ssh session failed: %v", err)
	}
	defer session.Close()

	var stdout, stderr io.Reader
	stdout, err = session.StdoutPipe()
	if err != nil {
		return fail("stdout pipe: %v", err)
	}
	stderr, err = session.StderrPipe()
	if err != nil {
		return fail("stderr pipe: %v", err)
	}

	if err := session.Start(command); err != nil {
		return fail("command start failed: %v", err)
	}

	outBytes, _ := io.ReadAll(io.LimitReader(stdout, outputLimit))
	errBytes, _ := io.ReadAll(io.LimitReader(stderr, outputLimit))

	exitCode := 0
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		_ = session.Close()
		return Result{
			Stdout:     string(outBytes),
			Stderr:     string(errBytes),
			Error:      ctx.Err().Error(),
			ExitCode:   -1,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
	if waitErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = -1
		}
	}

	return Result{
		Stdout:     string(outBytes),
		Stderr:     string(errBytes),
		ExitCode:   exitCode,
		DurationMs: time.Since(start).Milliseconds(),
	}
}
