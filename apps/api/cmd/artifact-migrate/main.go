package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pgd1001/svrtools/packages/artifacts"
	"github.com/pgd1001/svrtools/packages/config"
)

func main() {
	artifactsDir := flag.String("artifacts", "", "local artifact directory, defaults to ARTIFACTS_DIR")
	force := flag.Bool("force", false, "replace S3 objects whose checksum differs from the local source")
	manifestPath := flag.String("manifest", "", "write a verified destination manifest to this path")
	verifyManifestPath := flag.String("verify-manifest", "", "verify an existing destination manifest instead of migrating")
	restoreManifestPath := flag.String("restore-manifest", "", "restore an S3 manifest into encrypted local storage")
	restoreArtifactsDir := flag.String("restore-artifacts", "", "local restore directory, defaults to ARTIFACTS_DIR")
	flag.Parse()

	cfg := config.Load()
	if *artifactsDir != "" {
		cfg.ArtifactsDir = *artifactsDir
	}
	remote, err := artifacts.NewS3Store(cfg.S3Config())
	if err != nil {
		fatal(err)
	}
	if *verifyManifestPath != "" {
		manifest, err := artifacts.ReadManifest(*verifyManifestPath)
		if err != nil {
			fatal(err)
		}
		if err := artifacts.VerifyS3Manifest(remote, manifest); err != nil {
			fatal(err)
		}
		fmt.Printf("artifact manifest verified: %s (%d objects)\n", *verifyManifestPath, len(manifest.Entries))
		return
	}
	if *restoreManifestPath != "" {
		manifest, err := artifacts.ReadManifest(*restoreManifestPath)
		if err != nil {
			fatal(err)
		}
		destination := cfg.ArtifactsDir
		if *restoreArtifactsDir != "" {
			destination = *restoreArtifactsDir
		}
		local, err := artifacts.NewLocalStore(destination, cfg.ArtifactKey)
		if err != nil {
			fatal(err)
		}
		report, err := artifacts.RestoreS3Manifest(remote, manifest, local)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("artifact restore complete: restored=%d skipped=%d directory=%s\n", report.Restored, report.Skipped, destination)
		return
	}
	local, err := artifacts.NewLocalStore(cfg.ArtifactsDir, cfg.ArtifactKey)
	if err != nil {
		fatal(err)
	}
	report, err := artifacts.MigrateLocalToS3(local, remote, artifacts.MigrationOptions{Force: *force})
	if err != nil {
		fatal(err)
	}
	if *manifestPath != "" {
		if err := artifacts.WriteManifest(*manifestPath, report.Manifest()); err != nil {
			fatal(err)
		}
	}
	fmt.Printf("artifact migration complete: copied=%d skipped=%d bytes=%d\n", report.Copied, report.Skipped, report.ScannedBytes)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
