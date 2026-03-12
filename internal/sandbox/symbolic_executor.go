package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type SymbolicCommand struct {
	Command string
	Args    []string
}

type SymbolicCommandExecutor struct {
	allowed map[string]SymbolicCommand
}

func DefaultSymbolicCommands() map[string]SymbolicCommand {
	return map[string]SymbolicCommand{
		"go_test_all":       {Command: "go", Args: []string{"test", "./..."}},
		"go_test_internal":  {Command: "go", Args: []string{"test", "./internal/..."}},
		"go_test_policy":    {Command: "go", Args: []string{"test", "./internal/policy", "-run", "TestPolicyEngine_AllowRead"}},
		"go_version":        {Command: "go", Args: []string{"version"}},
		"go_vet_all":        {Command: "go", Args: []string{"vet", "./..."}},
		"go_fmt_all":        {Command: "go", Args: []string{"fmt", "./..."}},
		"golangci_lint_run": {Command: "golangci-lint", Args: []string{"run"}},
	}
}

func NewSymbolicCommandExecutor(allowed map[string]SymbolicCommand) *SymbolicCommandExecutor {
	cp := make(map[string]SymbolicCommand, len(allowed))
	for name, spec := range allowed {
		cp[name] = spec
	}
	return &SymbolicCommandExecutor{allowed: cp}
}

func (e *SymbolicCommandExecutor) Run(ctx context.Context, command string, _ []string, options SubprocessOptions) (ExecuteResult, error) {
	spec, ok := e.allowed[strings.TrimSpace(command)]
	if !ok {
		return ExecuteResult{}, fmt.Errorf("%w: %s", ErrCommandNotAllowed, command)
	}

	cmdCtx := ctx
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		cmdCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(cmdCtx, spec.Command, spec.Args...) // #nosec G204 -- command and args come from a validated allowlist
	if options.WorkingDir != "" {
		cmd.Dir = options.WorkingDir
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result := ExecuteResult{ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	if cmdCtx.Err() == context.DeadlineExceeded {
		return ExecuteResult{}, fmt.Errorf("command timed out: %w", cmdCtx.Err())
	}
	return ExecuteResult{}, err
}

func RestrictedShellByTrustHandler(resolveTier func(context.Context, string) (string, error), l1 *L1Runner, l2 *L2Runner) tools.ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		cmd, _ := input["command"].(string)
		workDir, _ := input["work_dir"].(string)
		sessionID, _ := input["session_id"].(string)

		tier := "tier1"
		if resolveTier != nil {
			resolved, err := resolveTier(ctx, sessionID)
			if err == nil && resolved != "" {
				tier = resolved
			}
		}

		req := ExecuteRequest{
			Command:         cmd,
			WorkingDir:      workDir,
			Timeout:         120 * time.Second,
			MaxOutputBytes:  1024 * 1024,
			SideEffectClass: SideEffectShell,
			NetworkPolicy:   NetworkPolicyAllowlisted,
		}

		runner := selectRunnerForTrustTier(tier, l1, l2)
		res, err := runner.Execute(ctx, req)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"exit_code": res.ExitCode,
			"stdout":    res.Stdout,
			"stderr":    res.Stderr,
		}, nil
	}
}
