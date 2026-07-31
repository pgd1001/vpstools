package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

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
	j := &job{
		Command:            "uptime",
		CredentialRef:      "web-prod",
		HostKeyFingerprint: "SHA256:" + strings.Repeat("A", 43),
	}

	result := runTarget(context.Background(), sshcreds.NewKeystore(dir), j, "host.example.com", 22, "operator")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "credential unavailable") {
		t.Fatalf("expected a credential resolution failure, got %q", result.Error)
	}
}

// The refusal tests above would all pass if runTarget simply never connected.
// This proves the success path works: a key from the keystore authenticates,
// the pinned fingerprint matches the host, and the command's output comes back.
func TestRunTargetExecutesWithKeystoreCredentialAndPinnedHostKey(t *testing.T) {
	hostKey := generateSigner(t)
	address := startTestSSHServer(t, hostKey)
	host, port := splitHostPort(t, address)

	dir := t.TempDir()
	clientKey := marshalPrivateKey(t, generateEd25519(t))
	if err := os.WriteFile(filepath.Join(dir, "web-prod"), clientKey, 0o600); err != nil {
		t.Fatal(err)
	}

	j := &job{
		Command:            "uptime",
		CredentialRef:      "web-prod",
		HostKeyFingerprint: ssh.FingerprintSHA256(hostKey.PublicKey()),
	}
	result := runTarget(context.Background(), sshcreds.NewKeystore(dir), j, host, port, "operator")

	if result.Error != "" {
		t.Fatalf("unexpected error on the success path: %s", result.Error)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}
	if result.Stdout != "executed" {
		t.Fatalf("expected the command output, got %q", result.Stdout)
	}
}

// A host presenting a different key than the one recorded must not receive the
// command, even when the credential resolves correctly.
func TestRunTargetRefusesHostPresentingADifferentKey(t *testing.T) {
	address := startTestSSHServer(t, generateSigner(t))
	host, port := splitHostPort(t, address)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "web-prod"), marshalPrivateKey(t, generateEd25519(t)), 0o600); err != nil {
		t.Fatal(err)
	}

	// A fingerprint for some other host entirely.
	j := &job{
		Command:            "uptime",
		CredentialRef:      "web-prod",
		HostKeyFingerprint: ssh.FingerprintSHA256(generateSigner(t).PublicKey()),
	}
	result := runTarget(context.Background(), sshcreds.NewKeystore(dir), j, host, port, "operator")

	if result.ExitCode != -1 {
		t.Fatalf("expected refusal exit code -1, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Error, "host key verification failed") {
		t.Fatalf("expected a host key verification failure, got %q", result.Error)
	}
	if result.Stdout != "" {
		t.Fatalf("the command ran against an unverified host, output %q", result.Stdout)
	}
}

func generateEd25519(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return private
}

func generateSigner(t *testing.T) ssh.Signer {
	t.Helper()
	signer, err := ssh.NewSignerFromKey(generateEd25519(t))
	if err != nil {
		t.Fatalf("signer from key: %v", err)
	}
	return signer
}

func marshalPrivateKey(t *testing.T, key ed25519.PrivateKey) []byte {
	t.Helper()
	block, err := ssh.MarshalPrivateKey(key, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return pem.EncodeToMemory(block)
}

func splitHostPort(t *testing.T, address string) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split address: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return host, port
}

// startTestSSHServer runs a minimal SSH server that accepts any client and
// returns fixed output. A real handshake is used so the host key check is
// exercised end to end rather than mocked.
func startTestSSHServer(t *testing.T, hostKey ssh.Signer) string {
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
			go serveTestConnection(conn, config)
		}
	}()
	return listener.Addr().String()
}

func serveTestConnection(conn net.Conn, config *ssh.ServerConfig) {
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
