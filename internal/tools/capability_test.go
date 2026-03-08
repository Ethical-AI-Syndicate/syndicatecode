package tools

import (
	"context"
	"errors"
	"testing"
)

func TestToolCapabilityValidate(t *testing.T) {
	t.Parallel()

	valid := ToolCapability{
		Name:            "read_file",
		Version:         1,
		SideEffectClass: SideEffectRead,
		IsolationLevel:  IsolationLevel0,
		FilesystemScope: FilesystemScopeRepoOnly,
		NetworkClass:    NetworkNone,
		Limits: ExecutionLimits{
			TimeoutSeconds: 30,
			MaxOutputBytes: 1024,
		},
	}

	tests := []struct {
		name    string
		mutate  func(c *ToolCapability)
		wantErr error
	}{
		{
			name: "missing name",
			mutate: func(c *ToolCapability) {
				c.Name = ""
			},
			wantErr: ErrInvalidToolCapability,
		},
		{
			name: "missing isolation level",
			mutate: func(c *ToolCapability) {
				c.IsolationLevel = ""
			},
			wantErr: ErrInvalidToolCapability,
		},
		{
			name: "missing filesystem scope",
			mutate: func(c *ToolCapability) {
				c.FilesystemScope = ""
			},
			wantErr: ErrInvalidToolCapability,
		},
		{
			name: "missing network class",
			mutate: func(c *ToolCapability) {
				c.NetworkClass = ""
			},
			wantErr: ErrInvalidToolCapability,
		},
		{
			name: "missing timeout",
			mutate: func(c *ToolCapability) {
				c.Limits.TimeoutSeconds = 0
			},
			wantErr: ErrInvalidToolCapability,
		},
		{
			name: "missing output limit",
			mutate: func(c *ToolCapability) {
				c.Limits.MaxOutputBytes = 0
			},
			wantErr: ErrInvalidToolCapability,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			tc.mutate(&candidate)

			err := candidate.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}

	t.Run("valid capability", func(t *testing.T) {
		t.Parallel()
		if err := valid.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}

func TestRegistryRegisterRejectsInvalidCapability(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	err := r.Register(ToolCapability{Name: "bad"})
	if !errors.Is(err, ErrInvalidToolCapability) {
		t.Fatalf("expected %v, got %v", ErrInvalidToolCapability, err)
	}
}

func TestExecutorExecuteRejectsToolWithMissingMetadata(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	bad := ToolCapability{
		Name:            "dangerous_shell",
		Version:         1,
		SideEffectClass: SideEffectShell,
	}
	if err := r.RegisterUnchecked(bad); err != nil {
		t.Fatalf("expected unchecked registration to succeed, got %v", err)
	}

	exec := NewExecutor(r, StubRunner{})
	_, err := exec.Execute(context.Background(), "dangerous_shell", map[string]any{"cmd": "rm -rf /"})
	if !errors.Is(err, ErrInvalidToolCapability) {
		t.Fatalf("expected %v, got %v", ErrInvalidToolCapability, err)
	}
}
