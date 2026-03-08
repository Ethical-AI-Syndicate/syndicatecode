package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrIsolationViolation = errors.New("isolation policy violation")

type SideEffectClass string

const (
	SideEffectRead  SideEffectClass = "read"
	SideEffectWrite SideEffectClass = "write"
	SideEffectShell SideEffectClass = "shell"
)

type NetworkPolicy string

const (
	NetworkPolicyDisabled    NetworkPolicy = "disabled"
	NetworkPolicyAllowlisted NetworkPolicy = "allowlisted"
	NetworkPolicyEnabled     NetworkPolicy = "enabled"
)

type IsolationLevel string

const (
	IsolationLevel0 IsolationLevel = "l0"
	IsolationLevel1 IsolationLevel = "l1"
	IsolationLevel2 IsolationLevel = "l2"
)

type ExecuteRequest struct {
	Command         string
	Args            []string
	WorkingDir      string
	Env             map[string]string
	Timeout         time.Duration
	MaxOutputBytes  int
	SideEffectClass SideEffectClass
	NetworkPolicy   NetworkPolicy
}

type ExecuteResult struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	InProcess       bool
	StdoutTruncated bool
	StderrTruncated bool
}

type SubprocessOptions struct {
	WorkingDir     string
	Env            map[string]string
	Timeout        time.Duration
	MaxOutputBytes int
	NetworkPolicy  NetworkPolicy
	UseShell       bool
}

type CommandExecutor interface {
	Run(ctx context.Context, command string, args []string, options SubprocessOptions) (ExecuteResult, error)
}

type L0Runner struct {
	executor CommandExecutor
}

func NewL0Runner(executor CommandExecutor) *L0Runner {
	return &L0Runner{executor: executor}
}

func (r *L0Runner) Execute(_ context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if req.SideEffectClass != SideEffectRead {
		return ExecuteResult{}, fmt.Errorf("%w: l0 supports read-only actions", ErrIsolationViolation)
	}

	return ExecuteResult{ExitCode: 0, InProcess: true}, nil
}

type L1Runner struct {
	repoRoot   string
	allowedEnv []string
	executor   CommandExecutor
}

func NewL1Runner(repoRoot string, allowedEnv []string, executor CommandExecutor) *L1Runner {
	return &L1Runner{repoRoot: filepath.Clean(repoRoot), allowedEnv: append([]string{}, allowedEnv...), executor: executor}
}

func (r *L1Runner) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if err := validateWorkingDirUnderRepo(r.repoRoot, req.WorkingDir); err != nil {
		return ExecuteResult{}, err
	}
	if isShellBinary(req.Command) {
		return ExecuteResult{}, fmt.Errorf("%w: shell binaries are forbidden in l1", ErrIsolationViolation)
	}
	if req.NetworkPolicy == NetworkPolicyEnabled {
		return ExecuteResult{}, fmt.Errorf("%w: open network is forbidden in l1", ErrIsolationViolation)
	}

	options := SubprocessOptions{
		WorkingDir:     req.WorkingDir,
		Env:            filterEnv(req.Env, r.allowedEnv),
		Timeout:        req.Timeout,
		MaxOutputBytes: req.MaxOutputBytes,
		NetworkPolicy:  req.NetworkPolicy,
		UseShell:       false,
	}

	return r.executor.Run(ctx, req.Command, req.Args, options)
}

type L2Runner struct {
	repoRoot   string
	allowedEnv []string
	executor   CommandExecutor
}

func NewL2Runner(repoRoot string, allowedEnv []string, executor CommandExecutor) *L2Runner {
	return &L2Runner{repoRoot: filepath.Clean(repoRoot), allowedEnv: append([]string{}, allowedEnv...), executor: executor}
}

func (r *L2Runner) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if err := validateWorkingDirUnderRepo(r.repoRoot, req.WorkingDir); err != nil {
		return ExecuteResult{}, err
	}

	options := SubprocessOptions{
		WorkingDir:     req.WorkingDir,
		Env:            filterEnv(req.Env, r.allowedEnv),
		Timeout:        req.Timeout,
		MaxOutputBytes: req.MaxOutputBytes,
		NetworkPolicy:  req.NetworkPolicy,
		UseShell:       true,
	}

	return r.executor.Run(ctx, req.Command, req.Args, options)
}

func validateWorkingDirUnderRepo(repoRoot, dir string) error {
	if dir == "" {
		return fmt.Errorf("%w: working directory is required", ErrIsolationViolation)
	}
	repoClean := filepath.Clean(repoRoot)
	dirClean := filepath.Clean(dir)
	if dirClean != repoClean && !strings.HasPrefix(dirClean, repoClean+string(filepath.Separator)) {
		return fmt.Errorf("%w: working directory %q is outside repo root %q", ErrIsolationViolation, dir, repoRoot)
	}
	return nil
}

func isShellBinary(command string) bool {
	shellBinaries := []string{"sh", "bash", "zsh", "fish", "cmd", "powershell", "pwsh"}
	return slices.Contains(shellBinaries, strings.ToLower(strings.TrimSpace(command)))
}

func filterEnv(in map[string]string, allowlist []string) map[string]string {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, key := range allowlist {
		allowed[key] = struct{}{}
	}
	filtered := make(map[string]string)
	for key, value := range in {
		if _, ok := allowed[key]; ok {
			filtered[key] = value
		}
	}
	return filtered
}
