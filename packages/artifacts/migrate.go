package artifacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrationOptions controls a local-to-S3 artifact migration.
type MigrationOptions struct {
	// Force permits replacing an S3 object whose checksum differs from the
	// local source. It does not delete objects that are absent from the local store.
	Force bool
}

// MigrationReport describes the work performed by MigrateLocalToS3.
type MigrationReport struct {
	ScannedBytes int64
	Copied       int
	Skipped      int
	Entries      []ManifestEntry
}

// Manifest is a durable inventory of the objects expected in the destination
// store. It is intentionally independent of database metadata so it can be
// checked during an expand-and-contract migration and kept with backups.
type Manifest struct {
	Version   int             `json:"version"`
	CreatedAt string          `json:"created_at"`
	Entries   []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	ID          string `json:"id"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type RestoreReport struct {
	Restored int
	Skipped  int
}

func (r MigrationReport) Manifest() Manifest {
	entries := append([]ManifestEntry(nil), r.Entries...)
	return Manifest{Version: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), Entries: entries}
}

// VerifyS3Manifest reads every object listed in manifest and verifies its
// stable ID, size, and plaintext checksum. It does not delete unlisted
// objects, making it safe to run before a migration cutover.
func VerifyS3Manifest(destination *S3Store, manifest Manifest) error {
	if destination == nil {
		return errors.New("S3 destination is required")
	}
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported artifact manifest version %d", manifest.Version)
	}
	seen := make(map[string]bool, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if seen[entry.ID] {
			return fmt.Errorf("artifact manifest contains duplicate ID %q", entry.ID)
		}
		seen[entry.ID] = true
		data, meta, err := destination.Get(entry.ID)
		if err != nil {
			return fmt.Errorf("verify S3 artifact %q: %w", entry.ID, err)
		}
		if meta.Size != entry.Size || meta.SHA256 != entry.SHA256 {
			return fmt.Errorf("S3 manifest mismatch for %q: got size=%d sha256=%s, want size=%d sha256=%s", entry.ID, meta.Size, meta.SHA256, entry.Size, entry.SHA256)
		}
		if len(data) != int(entry.Size) {
			return fmt.Errorf("S3 manifest byte count mismatch for %q", entry.ID)
		}
	}
	return nil
}

// RestoreS3Manifest downloads verified plaintext objects into encrypted local
// storage. Existing local objects with the expected checksum are skipped.
// Stable IDs are preserved, so database references remain valid during
// rollback to the self-contained tier.
func RestoreS3Manifest(source *S3Store, manifest Manifest, destination *LocalStore) (RestoreReport, error) {
	if source == nil || destination == nil {
		return RestoreReport{}, errors.New("S3 source and local destination are required")
	}
	if err := VerifyS3Manifest(source, manifest); err != nil {
		return RestoreReport{}, err
	}
	var report RestoreReport
	for _, entry := range manifest.Entries {
		if data, meta, err := destination.Get(entry.ID); err == nil && meta.SHA256 == entry.SHA256 && meta.Size == entry.Size && len(data) == int(entry.Size) {
			report.Skipped++
			continue
		}
		data, meta, err := source.Get(entry.ID)
		if err != nil {
			return report, fmt.Errorf("read S3 artifact %q for restore: %w", entry.ID, err)
		}
		if meta.SHA256 != entry.SHA256 || meta.Size != entry.Size {
			return report, fmt.Errorf("S3 artifact %q changed during restore", entry.ID)
		}
		if _, err := destination.Put(entry.ID, entry.ContentType, data); err != nil {
			return report, fmt.Errorf("write local artifact %q: %w", entry.ID, err)
		}
		report.Restored++
	}
	return report, nil
}

func WriteManifest(path string, manifest Manifest) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("manifest path is required")
	}
	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-manifest-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(contents, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ReadManifest(path string) (Manifest, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode artifact manifest: %w", err)
	}
	return manifest, nil
}

// MigrateLocalToS3 copies decrypted local artifacts to S3, preserving their
// IDs. Existing objects with the same checksum are skipped. Each copied object
// is read back from S3 and its checksum is compared before the operation is
// reported as successful.
func MigrateLocalToS3(source *LocalStore, destination *S3Store, options MigrationOptions) (MigrationReport, error) {
	if source == nil || destination == nil {
		return MigrationReport{}, errors.New("local and S3 stores are required")
	}
	entries, err := os.ReadDir(source.root)
	if err != nil {
		return MigrationReport{}, fmt.Errorf("read local artifact directory: %w", err)
	}
	var report MigrationReport
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return report, fmt.Errorf("symlinks are not supported: %s", entry.Name())
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".bin") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".bin")
		if err := validateArtifactID(id); err != nil {
			return report, fmt.Errorf("local artifact %q: %w", entry.Name(), err)
		}
		data, meta, err := source.Get(id)
		if err != nil {
			return report, fmt.Errorf("read local artifact %q: %w", id, err)
		}
		report.ScannedBytes += meta.Size

		remote, remoteMeta, getErr := destination.Get(id)
		if getErr == nil {
			if remoteMeta.SHA256 == meta.SHA256 && len(remote) == len(data) {
				report.Skipped++
				report.Entries = append(report.Entries, ManifestEntry{ID: id, ContentType: meta.ContentType, Size: meta.Size, SHA256: meta.SHA256})
				continue
			}
			if !options.Force {
				return report, fmt.Errorf("S3 artifact %q conflicts with local checksum %s", id, meta.SHA256)
			}
		} else if !errors.Is(getErr, ErrNotFound) {
			return report, fmt.Errorf("inspect S3 artifact %q: %w", id, getErr)
		}

		if _, err := destination.Put(id, meta.ContentType, data); err != nil {
			return report, fmt.Errorf("copy artifact %q: %w", id, err)
		}
		verified, verifiedMeta, err := destination.Get(id)
		if err != nil {
			return report, fmt.Errorf("verify copied artifact %q: %w", id, err)
		}
		if verifiedMeta.SHA256 != meta.SHA256 || len(verified) != len(data) {
			return report, fmt.Errorf("verification checksum mismatch for artifact %q", id)
		}
		report.Copied++
		report.Entries = append(report.Entries, ManifestEntry{ID: id, ContentType: meta.ContentType, Size: meta.Size, SHA256: meta.SHA256})
	}
	return report, nil
}
