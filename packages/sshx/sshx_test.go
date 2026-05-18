package sshx

import (
	"context"
	"testing"
)

func TestExecutor_nonExistentHost(t *testing.T) {
	e := NewExecutor("nonexistent:22", "test", "test")
	result := e.Run(context.Background(), "echo hello")
	if result.ExitCode != -1 {
		t.Errorf("expected exit code -1, got %d", result.ExitCode)
	}
	if result.Error == "" {
		t.Error("expected error string, got empty")
	}
}
