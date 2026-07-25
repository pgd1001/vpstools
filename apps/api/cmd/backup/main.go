package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type manifest struct {
	CreatedAt     string `json:"created_at"`
	Database      string `json:"database"`
	ArtifactsDir  string `json:"artifacts_dir"`
	ArtifactFiles int    `json:"artifact_files"`
}

func main() {
	dbPath := flag.String("db", envOrDefault("DATABASE_URL", envOrDefault("DB_PATH", "./svrtools.db")), "SQLite database path")
	artifactsDir := flag.String("artifacts", envOrDefault("ARTIFACTS_DIR", envOrDefault("VPS_ARTIFACTS_DIR", "./data/artifacts")), "local artifact directory")
	outputDir := flag.String("output", "./backups/"+time.Now().UTC().Format("20060102T150405Z"), "backup directory")
	flag.Parse()

	if err := os.MkdirAll(*outputDir, 0700); err != nil {
		fatal(err)
	}
	db, err := sql.Open("sqlite", *dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	dbBackup := filepath.Join(*outputDir, "svrtools.db")
	if _, err := db.Exec("VACUUM INTO ?", dbBackup); err != nil {
		fatal(fmt.Errorf("database backup: %w", err))
	}

	count, err := copyTree(*artifactsDir, filepath.Join(*outputDir, "artifacts"))
	if err != nil {
		fatal(fmt.Errorf("artifact backup: %w", err))
	}
	m := manifest{CreatedAt: time.Now().UTC().Format(time.RFC3339), Database: "svrtools.db", ArtifactsDir: "artifacts", ArtifactFiles: count}
	contents, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(*outputDir, "manifest.json"), append(contents, '\n'), 0600); err != nil {
		fatal(err)
	}
	fmt.Printf("backup created at %s (%d artifact files)\n", *outputDir, count)
}

func copyTree(source, destination string) (int, error) {
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return 0, nil
	}
	count := 0
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if rel == ".key" || filepath.Ext(info.Name()) == ".bin" {
			if err := copyFile(path, target); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
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
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
