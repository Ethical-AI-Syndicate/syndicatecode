package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrCommandNotAllowed = errors.New("command not allowed")
	ErrWorkDirNotAllowed = errors.New("workdir not allowed")
)

type Config struct {
	RepoRoot       string
	AllowedCmds    map[string]struct{}
	DefaultTimeout time.Duration
	MaxOutputBytes int
}

type Request struct {
	Command string
	WorkDir string
	Env     map[string]string
	Timeout time.Duration
}

type Result struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	DurationMS      int64
	OutputTruncated bool
}

type Runner struct {
	cfg Config
}

func NewRunner(cfg Config) *Runner {
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 60 * time.Second
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = 512 * 1024
	}
	return &Runner{cfg: cfg}
}

func (r *Runner) Run(ctx context.Context, req Request) (*Result, error) {
	if req.Command == "" {
		return nil, fmt.Errorf("empty command: %w", ErrCommandNotAllowed)
	}
	if _, ok := r.cfg.AllowedCmds[req.Command]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrCommandNotAllowed, req.Command)
	}

	workDir := req.WorkDir
	if workDir == "" {
		workDir = r.cfg.RepoRoot
	}
	allowed, err := isWithinRoot(r.cfg.RepoRoot, workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to validate workdir: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("%w: %s", ErrWorkDirNotAllowed, workDir)
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = r.cfg.DefaultTimeout
	}
	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd, err := r.buildCommand(ctxTimeout, req.Command)
	if err != nil {
		return nil, err
	}
	cmd.Dir = workDir
	cmd.Env = r.filteredEnv(req.Env)

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	start := time.Now()
	err = cmd.Run()
	duration := time.Since(start).Milliseconds()

	stdout, stdoutTruncated := truncateString(stdoutBuf.String(), r.cfg.MaxOutputBytes)
	stderr, stderrTruncated := truncateString(stderrBuf.String(), r.cfg.MaxOutputBytes)

	result := &Result{
		ExitCode:        0,
		Stdout:          stdout,
		Stderr:          stderr,
		DurationMS:      duration,
		OutputTruncated: stdoutTruncated || stderrTruncated,
	}

	if err != nil {
		if exitErr := (&exec.ExitError{}); errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		if errors.Is(ctxTimeout.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("command timed out: %w", ctxTimeout.Err())
		}
		return nil, fmt.Errorf("failed to run command: %w", err)
	}

	return result, nil
}

func (r *Runner) buildCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	switch command {
	case "go_test_all":
		return exec.CommandContext(ctx, "go", "test", "./..."), nil
	case "go_test_internal":
		return exec.CommandContext(ctx, "go", "test", "./internal/..."), nil
	case "go_test_policy":
		return exec.CommandContext(ctx, "go", "test", "./internal/policy", "-run", "TestPolicyEngine_AllowRead"), nil
	case "go_version":
		return exec.CommandContext(ctx, "go", "version"), nil
	case "go_vet_all":
		return exec.CommandContext(ctx, "go", "vet", "./..."), nil
	case "go_fmt_all":
		return exec.CommandContext(ctx, "go", "fmt", "./..."), nil
	case "golangci_lint_run":
		return exec.CommandContext(ctx, "golangci-lint", "run"), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrCommandNotAllowed, command)
	}
}

func (r *Runner) filteredEnv(extra map[string]string) []string {
	allowPrefixes := []string{"PATH=", "HOME=", "TERM=", "GO"}

	base := make([]string, 0)
	for _, kv := range os.Environ() {
		key := kv
		if idx := strings.Index(kv, "="); idx > 0 {
			key = kv[:idx]
		}
		if isSensitiveKey(key) {
			continue
		}
		for _, p := range allowPrefixes {
			if strings.HasPrefix(kv, p) {
				base = append(base, kv)
				break
			}
		}
	}

	for k, v := range extra {
		if isSensitiveKey(k) {
			continue
		}
		base = append(base, fmt.Sprintf("%s=%s", k, v))
	}

	return base
}

func isSensitiveKey(key string) bool {
	upper := strings.ToUpper(key)
	sensitive := []string{"SECRET", "TOKEN", "PASSWORD", "KEY", "CREDENTIAL"}
	for _, s := range sensitive {
		if strings.Contains(upper, s) {
			return true
		}
	}
	return false
}

func isWithinRoot(root, dir string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		return false, err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return false, nil
	}
	return true, nil
}

func truncateString(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	return value[:maxBytes], true
}
