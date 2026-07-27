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
	flag.Parse()

	cfg := config.Load()
	if *artifactsDir != "" {
		cfg.ArtifactsDir = *artifactsDir
	}
	local, err := artifacts.NewLocalStore(cfg.ArtifactsDir, cfg.ArtifactKey)
	if err != nil {
		fatal(err)
	}
	remote, err := artifacts.NewS3Store(cfg.S3Config())
	if err != nil {
		fatal(err)
	}
	report, err := artifacts.MigrateLocalToS3(local, remote, artifacts.MigrationOptions{Force: *force})
	if err != nil {
		fatal(err)
	}
	fmt.Printf("artifact migration complete: copied=%d skipped=%d bytes=%d\n", report.Copied, report.Skipped, report.ScannedBytes)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
