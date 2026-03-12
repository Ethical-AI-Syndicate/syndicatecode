package controlplane

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

type fakeDelegateBroker struct {
	diagnosticsFn func(context.Context, string, string) ([]lspDiagnostic, error)
}

func (f fakeDelegateBroker) Diagnostics(ctx context.Context, repoPath, path string) ([]lspDiagnostic, error) {
	if f.diagnosticsFn == nil {
		return []lspDiagnostic{}, nil
	}
	return f.diagnosticsFn(ctx, repoPath, path)
}

func (fakeDelegateBroker) Symbols(context.Context, string, string) ([]lspSymbol, error) {
	return []lspSymbol{}, nil
}

func (fakeDelegateBroker) Hover(context.Context, string, lspPositionRequest) (lspHoverResponse, error) {
	return lspHoverResponse{}, nil
}

func (fakeDelegateBroker) Definition(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return []lspLocation{}, nil
}

func (fakeDelegateBroker) References(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return []lspLocation{}, nil
}

func (fakeDelegateBroker) Completions(context.Context, string, lspPositionRequest) ([]lspCompletionItem, error) {
	return []lspCompletionItem{}, nil
}

func TestManagedLSPBroker_MissingBinaryReturnsTypedError(t *testing.T) {
	broker := NewManagedLSPBroker(NoopLSPBroker{}, newLSPProcessSupervisor(
		func(string) (string, error) { return "", exec.ErrNotFound },
		nil,
	))

	_, err := broker.Diagnostics(context.Background(), t.TempDir(), "main.go")
	if err == nil {
		t.Fatal("expected error")
	}
	lspErr, ok := asLSPError(err)
	if !ok {
		t.Fatalf("expected LSPError, got %T", err)
	}
	if lspErr.Code != LSPErrorServerUnavailable {
		t.Fatalf("expected %s, got %s", LSPErrorServerUnavailable, lspErr.Code)
	}
	if lspErr.Retryable {
		t.Fatal("expected non-retryable error")
	}
}

func TestManagedLSPBroker_StartupFailureRetriesThenUnhealthy(t *testing.T) {
	attempts := 0
	broker := NewManagedLSPBroker(NoopLSPBroker{}, newLSPProcessSupervisor(
		func(string) (string, error) { return "/usr/bin/gopls", nil },
		func(context.Context, lspProcessConfig) error {
			attempts++
			return context.DeadlineExceeded
		},
	))

	_, err := broker.Diagnostics(context.Background(), t.TempDir(), "main.go")
	if err == nil {
		t.Fatal("expected startup error")
	}
	lspErr, ok := asLSPError(err)
	if !ok {
		t.Fatalf("expected LSPError, got %T", err)
	}
	if lspErr.Code != LSPErrorBackendUnhealthy {
		t.Fatalf("expected %s, got %s", LSPErrorBackendUnhealthy, lspErr.Code)
	}
	if attempts != 2 {
		t.Fatalf("expected two start attempts, got %d", attempts)
	}

	_, err = broker.Diagnostics(context.Background(), t.TempDir(), "main.go")
	if err == nil {
		t.Fatal("expected unhealthy backend error on subsequent request")
	}
	if attempts != 2 {
		t.Fatalf("expected no additional start attempts, got %d", attempts)
	}
}

func TestManagedLSPBroker_DiagnosticsTimeoutUsesCachedResult(t *testing.T) {
	call := 0
	delegate := fakeDelegateBroker{
		diagnosticsFn: func(context.Context, string, string) ([]lspDiagnostic, error) {
			call++
			if call == 1 {
				return []lspDiagnostic{{Path: "main.go", Severity: "error", Message: "x"}}, nil
			}
			return nil, newLSPError(LSPErrorRequestTimeout, 504, true, "lsp request timed out", nil)
		},
	}
	broker := NewManagedLSPBroker(delegate, newLSPProcessSupervisor(
		func(string) (string, error) { return "/usr/bin/gopls", nil },
		nil,
	))
	repo := t.TempDir()

	first, err := broker.Diagnostics(context.Background(), repo, "main.go")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected one diagnostic, got %d", len(first))
	}

	second, err := broker.Diagnostics(context.Background(), repo, "main.go")
	if err != nil {
		t.Fatalf("expected cached response, got error: %v", err)
	}
	if len(second) != 1 || second[0].Message != "x" {
		t.Fatalf("unexpected cached diagnostics: %+v", second)
	}
}

func TestAsLSPError_RejectsGenericErrors(t *testing.T) {
	if _, ok := asLSPError(errors.New("boom")); ok {
		t.Fatal("expected generic error not to match LSPError")
	}
}
