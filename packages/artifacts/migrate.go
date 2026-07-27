package artifacts

import (
	"errors"
	"fmt"
	"os"
	"strings"
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
	}
	return report, nil
}
