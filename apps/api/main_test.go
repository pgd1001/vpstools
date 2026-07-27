package main

import (
	"net/http/httptest"
	"testing"

	"github.com/pgd1001/svrtools/packages/artifacts"
	"github.com/pgd1001/svrtools/packages/config"
)

func TestNewArtifactStoreDefaultsToLocal(t *testing.T) {
	store, err := newArtifactStore(config.BackendConfig{
		ArtifactStore: "local", ArtifactsDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*artifacts.LocalStore); !ok {
		t.Fatalf("store type = %T, want local store", store)
	}
}

func TestNewArtifactStoreSelectsS3OnlyWithValidConfiguration(t *testing.T) {
	server := httptest.NewServer(nil)
	defer server.Close()
	store, err := newArtifactStore(config.BackendConfig{
		ArtifactStore: "s3", S3Endpoint: server.URL, S3Bucket: "artifacts",
		S3AccessKeyID: "access", S3SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*artifacts.S3Store); !ok {
		t.Fatalf("store type = %T, want S3 store", store)
	}

	if _, err := newArtifactStore(config.BackendConfig{ArtifactStore: "s3", S3Endpoint: "not-a-url", S3Bucket: "artifacts"}); err == nil {
		t.Fatal("expected malformed S3 setup to fail")
	}
}
