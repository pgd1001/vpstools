package artifacts

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestS3StoreRoundTripEncryptionAndCRUD(t *testing.T) {
	var mu sync.Mutex
	objects := map[string][]byte{}
	checksums := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		if r.Method == http.MethodHead && r.URL.Path == "/bucket" {
			w.WriteHeader(http.StatusOK)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			objects[key] = body
			checksums[key] = r.Header.Get("X-Amz-Meta-Sha256")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("X-Amz-Meta-Sha256", checksums[key])
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(body)
		case http.MethodDelete:
			if _, ok := objects[key]; !ok {
				http.NotFound(w, r)
				return
			}
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	key := base64.RawStdEncoding.EncodeToString(bytesOf(32, 0x2a))
	store, err := NewS3Store(S3Config{Endpoint: server.URL, Bucket: "bucket", Prefix: "artifacts", EncryptionKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Check(); err != nil {
		t.Fatal(err)
	}
	want := []byte("private output")
	meta, err := store.Put("out-1", "text/plain", want)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != int64(len(want)) || meta.ContentType != "text/plain" || meta.SHA256 == "" {
		t.Fatalf("bad metadata: %+v", meta)
	}
	mu.Lock()
	stored := append([]byte(nil), objects["artifacts/out-1"]...)
	mu.Unlock()
	if string(stored) == string(want) {
		t.Fatal("S3 object was stored in plaintext")
	}
	got, gotMeta, err := store.Get("out-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || gotMeta.SHA256 != meta.SHA256 {
		t.Fatalf("got %q, metadata %+v", got, gotMeta)
	}
	if err := store.Delete("out-1"); err != nil {
		t.Fatal(err)
	}
}

func TestS3StoreRetriesTransientFailureAndVerifiesChecksum(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		w.Header().Set("X-Amz-Meta-Sha256", "wrong")
		_, _ = w.Write([]byte("body"))
	}))
	defer server.Close()
	store, err := NewS3Store(S3Config{Endpoint: server.URL, Bucket: "bucket", MaxRetries: 1, RetryBackoff: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get("a"); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error after retry, got %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestMigrateLocalToS3IsIdempotentAndRefusesConflicts(t *testing.T) {
	objects := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/bucket/")
		switch r.Method {
		case http.MethodPut:
			objects[key] = mustReadBody(r)
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			body, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			hash := sha256.Sum256(body)
			w.Header().Set("X-Amz-Meta-Sha256", fmt.Sprintf("%x", hash))
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	local, err := NewLocalStore(filepath.Join(t.TempDir(), "local"), base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Put("one", "text/plain", []byte("local")); err != nil {
		t.Fatal(err)
	}
	remote, err := NewS3Store(S3Config{Endpoint: server.URL, Bucket: "bucket"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := MigrateLocalToS3(local, remote, MigrationOptions{})
	if err != nil || report.Copied != 1 || report.Skipped != 0 {
		t.Fatalf("first migration report=%+v err=%v", report, err)
	}
	report, err = MigrateLocalToS3(local, remote, MigrationOptions{})
	if err != nil || report.Copied != 0 || report.Skipped != 1 {
		t.Fatalf("second migration report=%+v err=%v", report, err)
	}
	objects["one"] = []byte("different")
	if _, err := MigrateLocalToS3(local, remote, MigrationOptions{}); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if report, err := MigrateLocalToS3(local, remote, MigrationOptions{Force: true}); err != nil || report.Copied != 1 {
		t.Fatalf("forced migration report=%+v err=%v", report, err)
	}
}

func mustReadBody(r *http.Request) []byte {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	return body
}

func TestNewS3StoreValidatesStartupConfiguration(t *testing.T) {
	cases := []S3Config{
		{Endpoint: "", Bucket: "bucket"},
		{Endpoint: "http://example.test", Bucket: ""},
		{Endpoint: "http://example.test", Bucket: "bucket", AccessKeyID: "only"},
		{Endpoint: "http://example.test", Bucket: "bucket", Timeout: -1},
		{Endpoint: "http://example.test", Bucket: "bucket", MaxRetries: 6},
		{Endpoint: "http://example.test", Bucket: "bucket", EncryptionKey: base64.RawStdEncoding.EncodeToString([]byte("short"))},
	}
	for i, cfg := range cases {
		if _, err := NewS3Store(cfg); err == nil {
			t.Errorf("case %d unexpectedly succeeded", i)
		}
	}
}

func TestS3StoreSignsAuthenticatedRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
			t.Error("missing SigV4 authorization")
		}
		if r.Header.Get("X-Amz-Content-Sha256") == "" || r.Header.Get("X-Amz-Date") == "" {
			t.Error("missing SigV4 headers")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store, err := NewS3Store(S3Config{Endpoint: server.URL, Bucket: "bucket", AccessKeyID: "access", SecretAccessKey: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put("signed", "", []byte("data")); err != nil {
		t.Fatal(err)
	}
}

func bytesOf(n int, value byte) []byte {
	return []byte(fmt.Sprintf("%s", strings.Repeat(string(value), n)))
}

func TestLocalStoreEncryptsAndRoundTrips(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	key := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	store, err := NewLocalStore(root, key)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("secret execution output")
	meta, err := store.Put("out_123", "text/plain", want)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Size != int64(len(want)) || meta.SHA256 == "" {
		t.Fatalf("bad metadata: %+v", meta)
	}
	raw, err := os.ReadFile(filepath.Join(root, "out_123.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == string(want) {
		t.Fatal("artifact was stored in plaintext")
	}
	got, _, err := store.Get("out_123")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLocalStoreGeneratesKey(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	if _, err := NewLocalStore(root, ""); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, ".key")); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		t.Fatalf("key permissions or file missing: %v", err)
	}
}

func TestLocalStoreCheckReadsEncryptedArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "artifacts")
	key := base64.RawStdEncoding.EncodeToString(make([]byte, 32))
	store, err := NewLocalStore(root, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Check(); err != nil {
		t.Fatalf("empty store check failed: %v", err)
	}
	if _, err := store.Put("probe", "text/plain", []byte("health")); err != nil {
		t.Fatal(err)
	}
	if err := store.Check(); err != nil {
		t.Fatalf("encrypted store check failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "probe.bin"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Check(); err == nil {
		t.Fatal("expected corrupted encrypted artifact to fail the store check")
	}
}

func TestLocalStoreRejectsUnsafeArtifactIDs(t *testing.T) {
	store, err := NewLocalStore(filepath.Join(t.TempDir(), "artifacts"), base64.RawStdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "../outside", `..\outside`, "/absolute", "a?b"} {
		t.Run(id, func(t *testing.T) {
			if _, err := store.Put(id, "text/plain", []byte("data")); err == nil {
				t.Fatal("unsafe artifact ID was accepted")
			}
			if _, _, err := store.Get(id); err == nil {
				t.Fatal("unsafe artifact ID was read")
			}
			if err := store.Delete(id); err == nil {
				t.Fatal("unsafe artifact ID was deleted")
			}
		})
	}
}
