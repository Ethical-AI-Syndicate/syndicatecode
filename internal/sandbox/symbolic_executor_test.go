package sandbox

import (
	"context"
	"testing"
)

func TestSymbolicExecutorRejectsUnknownCommand_Bead_l3d_17_1(t *testing.T) {
	executor := NewSymbolicCommandExecutor(DefaultSymbolicCommands())

	ctx := context.Background()
	_, err := executor.Run(ctx, "unknown_command", []string{}, SubprocessOptions{})

	if err == nil {
		t.Fatal("Run should reject unknown command")
	}

	if err != ErrCommandNotAllowed {
		// Check if the error wraps ErrCommandNotAllowed
		if err.Error() != "command not allowed: unknown_command" {
			t.Errorf("expected ErrCommandNotAllowed, got: %v", err)
		}
	}
}

func TestSymbolicExecutorAllowsKnownCommand_Bead_l3d_17_1(t *testing.T) {
	executor := NewSymbolicCommandExecutor(DefaultSymbolicCommands())

	ctx := context.Background()
	_, err := executor.Run(ctx, "go_version", []string{}, SubprocessOptions{})

	// This may fail due to working directory or other runtime issues,
	// but it should not fail due to the command being rejected as unknown.
	if err != nil && err == ErrCommandNotAllowed {
		t.Errorf("Run rejected a known command: %v", err)
	}
}
