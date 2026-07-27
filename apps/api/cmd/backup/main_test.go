package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupVerifyAndRestore(t *testing.T) {
	artifactKey := []byte(strings.Repeat("k", 32))
	t.Setenv("BACKUP_ENCRYPTION_KEY", base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("b", 32))))
	root := t.TempDir()
	dbPath := filepath.Join(root, "source.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE example (value TEXT); INSERT INTO example VALUES ('kept')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifacts, ".key"), artifactKey, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifacts, "output.bin"), []byte("binary-data"), 0600); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup")
	if err := createBackup(dbPath, artifacts, backup); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(backup, "artifacts", ".key")) {
		t.Fatal("backup contains the plaintext artifact key")
	}
	if !exists(filepath.Join(backup, "artifacts", ".key.enc")) {
		t.Fatal("backup does not contain the encrypted artifact key")
	}
	if err := verifyBackup(backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "unexpected.txt"), []byte("unlisted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup(backup); err == nil {
		t.Fatal("verify succeeded with an unlisted backup file")
	}
	if err := os.Remove(filepath.Join(backup, "unexpected.txt")); err != nil {
		t.Fatal(err)
	}

	keyCiphertext, err := os.ReadFile(filepath.Join(backup, "artifacts", ".key.enc"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "artifacts", ".key.enc"), append(keyCiphertext, []byte("tamper")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup(backup); err == nil {
		t.Fatal("verify succeeded after artifact tampering")
	}
	if err := os.WriteFile(filepath.Join(backup, "artifacts", ".key.enc"), keyCiphertext, 0600); err != nil {
		t.Fatal(err)
	}

	restoredDB := filepath.Join(root, "restored", "svrtools.db")
	restoredArtifacts := filepath.Join(root, "restored", "artifacts")
	if err := restoreBackup(backup, restoredDB, restoredArtifacts, false); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite", restoredDB)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := restored.QueryRow("SELECT value FROM example").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "kept" {
		t.Fatalf("restored value = %q", value)
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(restoredArtifacts, ".key")); err != nil || string(got) != string(artifactKey) {
		t.Fatalf("restored artifact = %q, err=%v", got, err)
	}
}

func TestBackupRequiresSeparateKeyForGeneratedArtifactKey(t *testing.T) {
	t.Setenv("BACKUP_ENCRYPTION_KEY", "")
	root := t.TempDir()
	dbPath := filepath.Join(root, "source.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE example (value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(root, "artifacts")
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifacts, ".key"), []byte(strings.Repeat("k", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := createBackup(dbPath, artifacts, filepath.Join(root, "backup")); err == nil || !strings.Contains(err.Error(), "BACKUP_ENCRYPTION_KEY") {
		t.Fatalf("expected separate backup key error, got %v", err)
	}
}

func TestValidateRelativePathRejectsTraversal(t *testing.T) {
	for _, path := range []string{"../outside", "..\\outside", "/absolute", "C:\\absolute"} {
		if err := validateRelativePath(path); err == nil {
			t.Errorf("validateRelativePath(%q) accepted traversal", path)
		}
	}
}

func TestVerifyRejectsForeignKeyViolations(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "source.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=OFF; CREATE TABLE parent (id INTEGER PRIMARY KEY); CREATE TABLE child (parent_id INTEGER REFERENCES parent(id)); INSERT INTO child VALUES (99)"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup")
	if err := createBackup(dbPath, filepath.Join(root, "artifacts"), backup); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup(backup); err == nil || !strings.Contains(err.Error(), "foreign_key_check") {
		t.Fatalf("expected foreign-key validation failure, got %v", err)
	}
}

func TestVerifyRejectsIncompleteManifest(t *testing.T) {
	root := t.TempDir()
	m := manifest{
		Version:      manifestVersion,
		CreatedAt:    "2026-07-27T12:00:00Z",
		Database:     "svrtools.db",
		ArtifactsDir: "artifacts",
		Files: []file{{
			Path:   "artifacts/output.bin",
			Size:   1,
			SHA256: strings.Repeat("0", 64),
		}},
	}
	contents, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackup(root); err == nil || !strings.Contains(err.Error(), "exactly one database") {
		t.Fatalf("expected incomplete manifest failure, got %v", err)
	}
}
