package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// newTestKey returns a signer usable both as a host key and as a client key.
func newTestKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("signer from key: %v", err)
	}
	return signer
}

// startSSHServer runs a minimal SSH server that accepts any client and reports
// the fixed output for whatever command it is given. It exists so the host key
// verification path can be tested against a real handshake rather than a mock,
// which is the only way to prove the pinning actually rejects a wrong key.
func startSSHServer(t *testing.T, hostKey ssh.Signer) string {
	t.Helper()
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(hostKey)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveConnection(conn, config)
		}
	}()
	return listener.Addr().String()
}

func serveConnection(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range channelRequests {
				if req.Type == "exec" {
					_ = req.Reply(true, nil)
					_, _ = channel.Write([]byte("executed"))
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					_ = channel.Close()
					return
				}
				_ = req.Reply(false, nil)
			}
		}()
	}
}

// TestRunRejectsMismatchedHostKey is the central guarantee of this package: a
// host presenting a key other than the pinned one must not receive the
// command. Without this the runner would happily execute privileged work
// against anything answering on the address.
func TestRunRejectsMismatchedHostKey(t *testing.T) {
	serverKey := newTestKey(t)
	address := startSSHServer(t, serverKey)

	otherKey := newTestKey(t)
	wrongFingerprint := FingerprintSHA256(otherKey.PublicKey())

	executor, err := NewExecutor(address, "operator", Credential{Password: "irrelevant"}, wrongFingerprint)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	result := executor.Run(context.Background(), "uptime")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "host key verification failed") {
		t.Fatalf("expected a host key verification failure, got %q", result.Error)
	}
	if result.Stdout != "" {
		t.Fatalf("command produced output despite a host key mismatch: %q", result.Stdout)
	}
}

// TestRunAcceptsPinnedHostKey proves the pinning is not simply rejecting
// everything, which would make the mismatch test meaningless.
func TestRunAcceptsPinnedHostKey(t *testing.T) {
	serverKey := newTestKey(t)
	address := startSSHServer(t, serverKey)

	executor, err := NewExecutor(address, "operator", Credential{Password: "irrelevant"}, FingerprintSHA256(serverKey.PublicKey()))
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	result := executor.Run(context.Background(), "uptime")

	if result.Error != "" {
		t.Fatalf("unexpected error against a correctly pinned host: %s", result.Error)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "executed" {
		t.Fatalf("expected command output, got %q", result.Stdout)
	}
}

// TestNewExecutorRequiresPinnedHostKey confirms there is no way to construct
// an executor that skips verification.
func TestNewExecutorRequiresPinnedHostKey(t *testing.T) {
	for _, fingerprint := range []string{"", "   "} {
		if _, err := NewExecutor("host:22", "operator", Credential{Password: "x"}, fingerprint); !errors.Is(err, ErrNoHostKeyPinned) {
			t.Fatalf("fingerprint %q: expected ErrNoHostKeyPinned, got %v", fingerprint, err)
		}
	}
}

// TestNewExecutorRequiresCredential confirms an empty credential is refused up
// front rather than producing a confusing authentication failure later.
func TestNewExecutorRequiresCredential(t *testing.T) {
	_, err := NewExecutor("host:22", "operator", Credential{}, "SHA256:"+strings.Repeat("A", 43))
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("expected ErrNoCredential, got %v", err)
	}
}

// TestPrivateKeyCredentialIsUsable proves key-based authentication is wired
// up, since replacing password auth is the point of the change.
func TestPrivateKeyCredentialIsUsable(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pem, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	methods, err := Credential{PrivateKey: encodePEM(t, pem)}.authMethods()
	if err != nil {
		t.Fatalf("auth methods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected one public key auth method, got %d", len(methods))
	}
}

// TestFingerprintMatchesOpenSSHForm keeps the stored value comparable with
// what an operator gets from ssh-keygen.
func TestFingerprintMatchesOpenSSHForm(t *testing.T) {
	key := newTestKey(t)
	got := FingerprintSHA256(key.PublicKey())
	want := ssh.FingerprintSHA256(key.PublicKey())
	if got != want {
		t.Fatalf("fingerprint %q does not match the OpenSSH form %q", got, want)
	}
}

// TestRunReportsDialFailure confirms an unreachable host is reported as a
// result rather than a panic, since the caller records every attempt.
func TestRunReportsDialFailure(t *testing.T) {
	executor, err := NewExecutor("127.0.0.1:1", "operator", Credential{Password: "x"}, "SHA256:"+strings.Repeat("A", 43))
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := executor.Run(ctx, "uptime")
	if result.ExitCode != -1 || result.Error == "" {
		t.Fatalf("expected a reported dial failure, got exit %d error %q", result.ExitCode, result.Error)
	}
}

func encodePEM(t *testing.T, block *pem.Block) []byte {
	t.Helper()
	return pem.EncodeToMemory(block)
}
