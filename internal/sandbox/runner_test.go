package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunner_RunAllowlistedCommand(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_version": {}},
		DefaultTimeout: 120 * time.Second,
		MaxOutputBytes: 4096,
	})

	result, err := runner.Run(context.Background(), Request{
		Command: "go_version",
		WorkDir: repoRoot,
	})
	if err != nil {
		t.Fatalf("expected allowed command to run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestRunner_DenyNonAllowlistedCommand(t *testing.T) {
	repoRoot := t.TempDir()
	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_version": {}},
		DefaultTimeout: 5 * time.Second,
		MaxOutputBytes: 4096,
	})

	_, err := runner.Run(context.Background(), Request{
		Command: "sh",
		WorkDir: repoRoot,
	})
	if err == nil {
		t.Fatal("expected non-allowlisted command to be denied")
	}
}

func TestRunner_DenyWorkDirEscape(t *testing.T) {
	repoRoot := t.TempDir()
	outside := t.TempDir()
	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_version": {}},
		DefaultTimeout: 5 * time.Second,
		MaxOutputBytes: 4096,
	})

	_, err := runner.Run(context.Background(), Request{
		Command: "go_version",
		WorkDir: outside,
	})
	if err == nil {
		t.Fatal("expected workdir escape to be denied")
	}
}

func TestRunner_FilterEnvRemovesSensitive(t *testing.T) {
	repoRoot := t.TempDir()
	runner := NewRunner(Config{
		RepoRoot:       repoRoot,
		AllowedCmds:    map[string]struct{}{"go_version": {}},
		DefaultTimeout: 5 * time.Second,
		MaxOutputBytes: 4096,
	})

	t.Setenv("AWS_SECRET_ACCESS_KEY", "very-secret")
	t.Setenv("PUBLIC_FLAG", "ok")

	env := runner.filteredEnv(map[string]string{
		"API_TOKEN":   "123",
		"SAFE_OPTION": "1",
	})

	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "AWS_SECRET_ACCESS_KEY") || strings.Contains(joined, "API_TOKEN") {
		t.Fatal("expected sensitive variables to be filtered")
	}
	if !strings.Contains(joined, "SAFE_OPTION=1") {
		t.Fatal("expected safe variable to be preserved")
	}
}
