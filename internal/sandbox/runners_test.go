package sandbox

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type fakeExecutor struct {
	called      bool
	lastCommand string
	lastArgs    []string
	lastOptions SubprocessOptions
	result      ExecuteResult
	err         error
}

func (f *fakeExecutor) Run(_ context.Context, command string, args []string, options SubprocessOptions) (ExecuteResult, error) {
	f.called = true
	f.lastCommand = command
	f.lastArgs = append([]string{}, args...)
	f.lastOptions = options
	if f.err != nil {
		return ExecuteResult{}, f.err
	}
	return f.result, nil
}

func TestL0RunnerAllowsReadOnlyInProcess(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{}
	runner := NewL0Runner(executor)

	result, err := runner.Execute(context.Background(), ExecuteRequest{
		SideEffectClass: SideEffectRead,
		Command:         "cat",
		Args:            []string{"README.md"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if executor.called {
		t.Fatalf("expected l0 to avoid subprocess execution")
	}
	if !result.InProcess {
		t.Fatalf("expected in-process result for l0")
	}
}

func TestL0RunnerRejectsSideEffectingRequests(t *testing.T) {
	t.Parallel()

	runner := NewL0Runner(&fakeExecutor{})
	_, err := runner.Execute(context.Background(), ExecuteRequest{SideEffectClass: SideEffectWrite})
	if !errors.Is(err, ErrIsolationViolation) {
		t.Fatalf("expected %v, got %v", ErrIsolationViolation, err)
	}
}

func TestL1RunnerRejectsShellBinary(t *testing.T) {
	t.Parallel()

	repoRoot := "/repo"
	runner := NewL1Runner(repoRoot, []string{"PATH"}, &fakeExecutor{})
	_, err := runner.Execute(context.Background(), ExecuteRequest{
		Command:         "bash",
		WorkingDir:      "/repo",
		SideEffectClass: SideEffectRead,
		NetworkPolicy:   NetworkPolicyDisabled,
	})
	if !errors.Is(err, ErrIsolationViolation) {
		t.Fatalf("expected %v, got %v", ErrIsolationViolation, err)
	}
}

func TestL1RunnerRejectsWorkingDirOutsideRepo(t *testing.T) {
	t.Parallel()

	repoRoot := "/repo"
	runner := NewL1Runner(repoRoot, []string{"PATH"}, &fakeExecutor{})
	_, err := runner.Execute(context.Background(), ExecuteRequest{
		Command:         "go",
		Args:            []string{"test", "./..."},
		WorkingDir:      "/other",
		SideEffectClass: SideEffectRead,
		NetworkPolicy:   NetworkPolicyDisabled,
	})
	if !errors.Is(err, ErrIsolationViolation) {
		t.Fatalf("expected %v, got %v", ErrIsolationViolation, err)
	}
}

func TestL1RunnerFiltersEnvironmentAndPassesOptions(t *testing.T) {
	t.Parallel()

	repoRoot := "/repo"
	executor := &fakeExecutor{result: ExecuteResult{ExitCode: 0}}
	runner := NewL1Runner(repoRoot, []string{"PATH", "HOME"}, executor)

	_, err := runner.Execute(context.Background(), ExecuteRequest{
		Command:         "go",
		Args:            []string{"test", "./..."},
		WorkingDir:      filepath.Join(repoRoot, "internal"),
		SideEffectClass: SideEffectRead,
		NetworkPolicy:   NetworkPolicyAllowlisted,
		Timeout:         10 * time.Second,
		Env: map[string]string{
			"PATH":          "/usr/bin",
			"HOME":          "/tmp/home",
			"SHOULD_FILTER": "1",
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !executor.called {
		t.Fatalf("expected subprocess to be called")
	}
	if executor.lastOptions.UseShell {
		t.Fatalf("expected l1 to disable shell")
	}
	if executor.lastOptions.NetworkPolicy != NetworkPolicyAllowlisted {
		t.Fatalf("expected allowlisted network policy, got %s", executor.lastOptions.NetworkPolicy)
	}
	if _, ok := executor.lastOptions.Env["SHOULD_FILTER"]; ok {
		t.Fatalf("expected environment filtering to remove non-allowlisted variables")
	}
}

func TestL2RunnerUsesShellAndRespectsExplicitControls(t *testing.T) {
	t.Parallel()

	repoRoot := "/repo"
	executor := &fakeExecutor{result: ExecuteResult{ExitCode: 0}}
	runner := NewL2Runner(repoRoot, []string{"PATH"}, executor)

	_, err := runner.Execute(context.Background(), ExecuteRequest{
		Command:         "echo hello",
		WorkingDir:      "/repo",
		SideEffectClass: SideEffectShell,
		NetworkPolicy:   NetworkPolicyEnabled,
		Timeout:         2 * time.Second,
		Env: map[string]string{
			"PATH":   "/usr/bin",
			"SECRET": "value",
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !executor.called {
		t.Fatalf("expected subprocess to be called")
	}
	if !executor.lastOptions.UseShell {
		t.Fatalf("expected l2 to run with shell enabled")
	}
	if executor.lastOptions.WorkingDir != "/repo" {
		t.Fatalf("expected explicit working directory, got %s", executor.lastOptions.WorkingDir)
	}
	if _, ok := executor.lastOptions.Env["SECRET"]; ok {
		t.Fatalf("expected l2 environment filtering to remove non-allowlisted variables")
	}
}
