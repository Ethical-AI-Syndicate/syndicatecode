package sandbox

import (
	"testing"
)

func TestDefaultSymbolicCommands_Bead_l3d_17_1(t *testing.T) {
	cmds := DefaultSymbolicCommands()

	expected := []string{
		"go_test_all",
		"go_test_internal",
		"go_test_policy",
		"go_version",
		"go_vet_all",
		"go_fmt_all",
		"golangci_lint_run",
	}

	for _, name := range expected {
		if _, ok := cmds[name]; !ok {
			t.Errorf("expected command %q to be defined", name)
		}
	}
}

func TestSymbolicCommandExecutor_New_Bead_l3d_17_1(t *testing.T) {
	allowed := map[string]SymbolicCommand{
		"test_cmd": {Command: "echo", Args: []string{"hello"}},
	}

	exec := NewSymbolicCommandExecutor(allowed)
	if exec == nil {
		t.Fatal("NewSymbolicCommandExecutor returned nil")
	}
	if exec.allowed == nil {
		t.Error("allowed map should not be nil")
	}
}

func TestSymbolicCommandExecutor_CopyOnConstruction_Bead_l3d_17_1(t *testing.T) {
	original := map[string]SymbolicCommand{
		"test_cmd": {Command: "echo", Args: []string{"hello"}},
	}

	exec := NewSymbolicCommandExecutor(original)

	original["test_cmd"] = SymbolicCommand{Command: "modified", Args: []string{}}

	if spec, ok := exec.allowed["test_cmd"]; !ok {
		t.Error("command should still exist after modifying original")
	} else if spec.Command == "modified" {
		t.Error("executor should have its own copy, not reference original")
	}
}
