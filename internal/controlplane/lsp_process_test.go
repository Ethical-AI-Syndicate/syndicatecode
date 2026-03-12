package controlplane

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestLSPProcessSupervisor_Bead_l3d_17_1(t *testing.T) {
	lookup := func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	starter := func(ctx context.Context, cfg lspProcessConfig) error {
		return nil
	}

	supervisor := newLSPProcessSupervisor(lookup, starter)

	detected := detectedLanguage{
		Language:   "python",
		Executable: "pylsp",
	}

	err := supervisor.EnsureStarted(context.Background(), detected)
	if err != nil {
		t.Errorf("EnsureStarted() error = %v", err)
	}
}

func TestNewLSPProcessSupervisor_Bead_l3d_17_1(t *testing.T) {
	supervisor := newLSPProcessSupervisor(nil, nil)
	if supervisor == nil {
		t.Fatal("newLSPProcessSupervisor() returned nil")
	}
	if supervisor.states == nil {
		t.Error("states map should be initialized")
	}
}

func TestIsMissingExecutable_Bead_l3d_17_1(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"exec.ErrNotFound", exec.ErrNotFound, true},
		{"os.ErrNotExist", os.ErrNotExist, true},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingExecutable(tt.err); got != tt.want {
				t.Errorf("isMissingExecutable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorString_Bead_l3d_17_1(t *testing.T) {
	if errorString(nil) != "" {
		t.Error("errorString(nil) should return empty string")
	}

	if errorString(nil) != "" {
		t.Error("errorString(nil) should return empty string")
	}

	customErr := &testError{"test error"}
	result := errorString(customErr)
	if result != "test error" {
		t.Errorf("errorString() = %v, want 'test error'", result)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestLSPProcessConfig_Bead_l3d_17_1(t *testing.T) {
	cfg := lspProcessConfig{
		Executable: "pylsp",
		Args:       []string{"--version"},
		Timeout:    5 * time.Second,
	}

	if cfg.Executable != "pylsp" {
		t.Errorf("Executable = %v, want 'pylsp'", cfg.Executable)
	}
}
