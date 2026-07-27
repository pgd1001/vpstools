package artifacts

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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
