package tools

import (
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()

	tool := ToolDefinition{
		Name:             "test_tool",
		Version:          "1",
		SideEffect:       SideEffectRead,
		ApprovalRequired: false,
		InputSchema: map[string]FieldSchema{
			"path": {Type: "string"},
		},
		OutputSchema: map[string]FieldSchema{
			"result": {Type: "string"},
		},
		Limits: ExecutionLimits{
			TimeoutSeconds: 30,
			MaxOutputBytes: 1000,
		},
	}

	err := reg.Register(tool)
	if err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	got, ok := reg.Get("test_tool")
	if !ok {
		t.Error("tool not found in registry")
	}
	if got.Name != tool.Name {
		t.Errorf("got %s, want %s", got.Name, tool.Name)
	}
}

func TestRegistry_Duplicate(t *testing.T) {
	reg := NewRegistry()

	tool := ToolDefinition{
		Name:         "duplicate",
		Version:      "1",
		SideEffect:   SideEffectNone,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits:       ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
	}

	_ = reg.Register(tool)
	err := reg.Register(tool)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()

	_ = reg.Register(ToolDefinition{
		Name:         "tool1",
		Version:      "1",
		SideEffect:   SideEffectRead,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits:       ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
	})
	_ = reg.Register(ToolDefinition{
		Name:         "tool2",
		Version:      "1",
		SideEffect:   SideEffectWrite,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits:       ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
		Security:     SecurityMetadata{FilesystemScope: "repo"},
	})

	tools := reg.List()
	if len(tools) != 2 {
		t.Errorf("got %d tools, want 2", len(tools))
	}
}
