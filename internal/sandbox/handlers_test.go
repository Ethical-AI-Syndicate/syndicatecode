package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunTestsHandler(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "../.."))

	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_test_policy": {}},
		DefaultTimeout: 120 * time.Second,
		MaxOutputBytes: 64 * 1024,
	})

	handler := RunTestsHandler(runner)
	result, err := handler(context.Background(), map[string]interface{}{
		"package": "./",
		"run":     "TestRunner_DenyWorkDirEscape",
	})
	if err != nil {
		t.Fatalf("run tests handler failed: %v", err)
	}
	if result["exit_code"].(int) != 0 {
		t.Fatalf("expected exit code 0, got %v, stderr=%v", result["exit_code"], result["stderr"])
	}
}

func TestRestrictedShellHandlerDeniesUnsafeCommand(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "../.."))

	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_test_policy": {}},
		DefaultTimeout: 120 * time.Second,
		MaxOutputBytes: 64 * 1024,
	})

	handler := RestrictedShellHandler(runner)
	_, err = handler(context.Background(), map[string]interface{}{
		"command": "sh",
	})
	if err == nil {
		t.Fatal("expected unsafe command to be denied")
	}
}
