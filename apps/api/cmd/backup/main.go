package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const manifestVersion = 1

var version = "dev"

type manifest struct {
	Version       int    `json:"version"`
	CreatedAt     string `json:"created_at"`
	Database      string `json:"database"`
	ArtifactsDir  string `json:"artifacts_dir"`
	ArtifactFiles int    `json:"artifact_files"`
	EncryptionKey string `json:"encryption_key,omitempty"`
	KeyRecovery   string `json:"key_recovery"`
	Files         []file `json:"files"`
}

type file struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	mode := flag.String("mode", "backup", "operation: backup, verify, or restore")
	dbPath := flag.String("db", envOrDefault("DATABASE_URL", envOrDefault("DB_PATH", "./svrtools.db")), "SQLite database path")
	artifactsDir := flag.String("artifacts", envOrDefault("ARTIFACTS_DIR", envOrDefault("VPS_ARTIFACTS_DIR", "./data/artifacts")), "local artifact directory")
	outputDir := flag.String("output", "./backups/"+time.Now().UTC().Format("20060102T150405Z"), "backup directory to create")
	inputDir := flag.String("input", "", "existing backup directory for verify or restore")
	force := flag.Bool("force", false, "restore over existing destinations, retaining them with a .pre-restore suffix")
	flag.Parse()

	var err error
	switch *mode {
	case "backup":
		err = createBackup(*dbPath, *artifactsDir, *outputDir)
	case "verify":
		if *inputDir == "" {
			err = errors.New("-input is required for verify")
		} else {
			err = verifyBackup(*inputDir)
		}
	case "restore":
		if *inputDir == "" {
			err = errors.New("-input is required for restore")
		} else {
			err = restoreBackup(*inputDir, *dbPath, *artifactsDir, *force)
		}
	default:
		err = fmt.Errorf("unsupported mode %q", *mode)
	}
	if err != nil {
		fatal(err)
	}
}

func createBackup(dbPath, artifactsDir, outputDir string) error {
	if unsupportedDatabaseURL(dbPath) {
		return fmt.Errorf("unsupported database %q: backup supports SQLite only", dbPath)
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".backup-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database: %w", err)
	}
	dbBackup := filepath.Join(tmp, "svrtools.db")
	if _, err := db.Exec("VACUUM INTO ?", dbBackup); err != nil {
		return fmt.Errorf("database backup: %w", err)
	}
	databaseFile, err := describeFile(dbBackup, "svrtools.db")
	if err != nil {
		return err
	}
	artifactFiles, err := copyTree(artifactsDir, filepath.Join(tmp, "artifacts"))
	if err != nil {
		return fmt.Errorf("artifact backup: %w", err)
	}
	files := append([]file{databaseFile}, artifactFiles...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	keyRecovery := "Restore artifacts/.key with the artifact directory, or provide the original ARTIFACT_ENCRYPTION_KEY from the configured secret manager before starting the API."
	encryptionKey := ""
	if _, err := os.Stat(filepath.Join(artifactsDir, ".key")); err == nil {
		encryptionKey = "artifacts/.key"
	}
	m := manifest{Version: manifestVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), Database: "svrtools.db", ArtifactsDir: "artifacts", ArtifactFiles: len(artifactFiles), EncryptionKey: encryptionKey, KeyRecovery: keyRecovery, Files: files}
	contents, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), append(contents, '\n'), 0600); err != nil {
		return err
	}
	if _, err := os.Stat(outputDir); err == nil {
		return fmt.Errorf("backup destination already exists: %s", outputDir)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmp, outputDir); err != nil {
		return fmt.Errorf("publish backup: %w", err)
	}
	fmt.Printf("backup created at %s (%d artifact files)\n", outputDir, len(artifactFiles))
	return nil
}

func verifyBackup(dir string) error {
	m, err := readManifest(dir)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if err := validateRelativePath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return fmt.Errorf("manifest contains duplicate path %q", f.Path)
		}
		seen[f.Path] = true
		actual, err := describeFile(filepath.Join(dir, filepath.FromSlash(f.Path)), f.Path)
		if err != nil {
			return err
		}
		if actual.Size != f.Size || actual.SHA256 != f.SHA256 {
			return fmt.Errorf("integrity check failed for %s", f.Path)
		}
	}
	if m.ArtifactFiles != countArtifactRecords(m) {
		return fmt.Errorf("manifest artifact count does not match file inventory")
	}
	var extras []string
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel != "manifest.json" && !seen[rel] {
			extras = append(extras, rel)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("inspect backup inventory: %w", err)
	}
	if len(extras) > 0 {
		sort.Strings(extras)
		return fmt.Errorf("backup contains files not listed in manifest: %s", strings.Join(extras, ", "))
	}
	fmt.Printf("backup verified: %s (%d files)\n", dir, len(m.Files))
	return nil
}

func restoreBackup(dir, dbTarget, artifactsTarget string, force bool) error {
	if err := verifyBackup(dir); err != nil {
		return err
	}
	m, _ := readManifest(dir)
	if unsupportedDatabaseURL(dbTarget) {
		return fmt.Errorf("unsupported database target %q: restore supports SQLite only", dbTarget)
	}
	if !force && (exists(dbTarget) || exists(artifactsTarget)) {
		return errors.New("restore destinations already exist; pass -force to retain them and replace them")
	}
	if err := os.MkdirAll(filepath.Dir(dbTarget), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(artifactsTarget), 0700); err != nil {
		return err
	}
	tmpDB := dbTarget + ".restore-tmp"
	defer os.Remove(tmpDB)
	if err := copyFile(filepath.Join(dir, m.Database), tmpDB); err != nil {
		return err
	}
	tmpArtifacts, err := os.MkdirTemp(filepath.Dir(artifactsTarget), ".artifacts-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpArtifacts)
	for _, f := range m.Files {
		if strings.HasPrefix(f.Path, m.ArtifactsDir+"/") {
			if err := copyFile(filepath.Join(dir, filepath.FromSlash(f.Path)), filepath.Join(tmpArtifacts, filepath.FromSlash(strings.TrimPrefix(f.Path, m.ArtifactsDir+"/")))); err != nil {
				return err
			}
		}
	}
	if force {
		oldDB, err := moveAside(dbTarget)
		if err != nil {
			return err
		}
		oldArtifacts, err := moveAside(artifactsTarget)
		if err != nil {
			_ = restoreAside(oldDB, dbTarget)
			return err
		}
		rollback := func() error {
			var rollbackErr error
			if exists(dbTarget) {
				rollbackErr = errors.Join(rollbackErr, os.RemoveAll(dbTarget))
			}
			if exists(artifactsTarget) {
				rollbackErr = errors.Join(rollbackErr, os.RemoveAll(artifactsTarget))
			}
			rollbackErr = errors.Join(rollbackErr, restoreAside(oldDB, dbTarget))
			rollbackErr = errors.Join(rollbackErr, restoreAside(oldArtifacts, artifactsTarget))
			return rollbackErr
		}
		if err := os.Rename(tmpDB, dbTarget); err != nil {
			return errors.Join(err, rollback())
		}
		if err := os.Rename(tmpArtifacts, artifactsTarget); err != nil {
			return errors.Join(err, rollback())
		}
	} else {
		if err := os.Rename(tmpDB, dbTarget); err != nil {
			return err
		}
		if err := os.Rename(tmpArtifacts, artifactsTarget); err != nil {
			_ = os.RemoveAll(dbTarget)
			return err
		}
	}
	checkDB, err := sql.Open("sqlite", dbTarget+"?_pragma=foreign_keys(on)")
	if err != nil {
		return fmt.Errorf("restored database validation: %w", err)
	}
	err = checkDB.Ping()
	_ = checkDB.Close()
	if err != nil {
		return fmt.Errorf("restored database validation: %w", err)
	}
	fmt.Printf("backup restored from %s\n", dir)
	return nil
}

func copyTree(source, destination string) ([]file, error) {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var files []file
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported: %s", path)
		}
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(destination, mustRel(source, path)), 0700)
		}
		if info.Name() != ".key" && filepath.Ext(info.Name()) != ".bin" {
			return nil
		}
		rel := mustRel(source, path)
		target := filepath.Join(destination, rel)
		if err := copyFile(path, target); err != nil {
			return err
		}
		described, err := describeFile(target, filepath.ToSlash(filepath.Join("artifacts", rel)))
		if err != nil {
			return err
		}
		files = append(files, described)
		return nil
	})
	return files, err
}

func describeFile(path, manifestPath string) (file, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return file{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return file{}, fmt.Errorf("symlinks are not supported: %s", path)
	}
	in, err := os.Open(path)
	if err != nil {
		return file{}, err
	}
	defer in.Close()
	h := sha256.New()
	n, err := io.Copy(h, in)
	if err != nil {
		return file{}, err
	}
	return file{Path: filepath.ToSlash(manifestPath), Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}
func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func readManifest(dir string) (manifest, error) {
	var m manifest
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return m, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version != manifestVersion || m.Database != "svrtools.db" || m.ArtifactsDir != "artifacts" {
		return m, errors.New("unsupported or incomplete backup manifest")
	}
	return m, nil
}
func validateRelativePath(p string) error {
	normalized := strings.ReplaceAll(p, "\\", "/")
	clean := path.Clean(normalized)
	if p == "" || strings.HasPrefix(normalized, "/") || filepath.IsAbs(filepath.FromSlash(normalized)) || clean != normalized || clean == ".." || strings.HasPrefix(clean, "../") || (len(clean) >= 2 && clean[1] == ':') {
		return fmt.Errorf("unsafe manifest path %q", p)
	}
	return nil
}
func countArtifactRecords(m manifest) int {
	n := 0
	for _, f := range m.Files {
		if strings.HasPrefix(f.Path, m.ArtifactsDir+"/") {
			n++
		}
	}
	return n
}
func mustRel(base, path string) string { r, _ := filepath.Rel(base, path); return r }
func exists(path string) bool          { _, err := os.Stat(path); return err == nil }
func moveAside(path string) (string, error) {
	if !exists(path) {
		return "", nil
	}
	backup := path + ".pre-restore-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	return backup, os.Rename(path, backup)
}
func restoreAside(backup, target string) error {
	if backup == "" || !exists(backup) {
		return nil
	}
	return os.Rename(backup, target)
}
func unsupportedDatabaseURL(path string) bool {
	return strings.HasPrefix(strings.ToLower(path), "postgres://") || strings.HasPrefix(strings.ToLower(path), "postgresql://") || strings.Contains(path, "host=")
}
func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
