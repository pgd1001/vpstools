package main

import (
	"database/sql"
	"encoding/base64"
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
