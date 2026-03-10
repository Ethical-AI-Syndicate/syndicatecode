package tools

import (
	"testing"
)

func TestToolDefinition_Validation(t *testing.T) {
	tool := ToolDefinition{
		Name:             "read_file",
		Version:          "1",
		SideEffect:       SideEffectRead,
		ApprovalRequired: false,
		InputSchema: map[string]FieldSchema{
			"path": {Type: "string", Description: "file path"},
		},
		OutputSchema: map[string]FieldSchema{
			"content": {Type: "string", Description: "file content"},
		},
		Limits: ExecutionLimits{
			TimeoutSeconds: 30,
			MaxOutputBytes: 500000,
		},
	}

	if err := tool.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestToolDefinition_InvalidName(t *testing.T) {
	tool := ToolDefinition{
		Name:    "",
		Version: "1",
	}

	err := tool.Validate()
	if err == nil {
		t.Error("expected validation error for empty name")
	}
}

// validToolDef returns a minimal valid ToolDefinition for use in subtests.
func validToolDef() ToolDefinition {
	return ToolDefinition{
		Name:         "test-tool",
		Version:      "1.0.0",
		SideEffect:   SideEffectNone,
		InputSchema:  map[string]FieldSchema{"msg": {Type: "string"}},
		OutputSchema: map[string]FieldSchema{"result": {Type: "string"}},
		Limits:       ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1024},
	}
}

func TestToolDefinition_ValidateRejectsUnknownSideEffect(t *testing.T) {
	t.Parallel()
	def := validToolDef()
	def.SideEffect = SideEffect("invalid_effect")
	if err := def.Validate(); err == nil {
		t.Fatal("expected error for unknown side effect, got nil")
	}
}

func TestToolDefinition_ValidateRejectsWriteSideEffectWithoutScope(t *testing.T) {
	t.Parallel()
	def := validToolDef()
	def.SideEffect = SideEffectWrite
	def.Security.FilesystemScope = ""
	if err := def.Validate(); err == nil {
		t.Fatal("expected error when write side-effect has no filesystem scope")
	}
}

func TestToolDefinition_ValidateAcceptsWriteWithScope(t *testing.T) {
	t.Parallel()
	def := validToolDef()
	def.SideEffect = SideEffectWrite
	def.Security.FilesystemScope = "repo"
	if err := def.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestToolDefinition_ValidateAcceptsNoneWithEmptyScope(t *testing.T) {
	t.Parallel()
	def := validToolDef()
	def.SideEffect = SideEffectNone
	def.Security.FilesystemScope = ""
	if err := def.Validate(); err != nil {
		t.Fatalf("unexpected error for none side-effect: %v", err)
	}
}

// TestToolDefinition_SideEffectAndScopeValidation_Bead_l3d_8_1 is the bead-tagged
// conformance entry point for l3d.8.1 (tool capability metadata and isolation mapping).
func TestToolDefinition_SideEffectAndScopeValidation_Bead_l3d_8_1(t *testing.T) {
	t.Parallel()
	t.Run("rejects unknown side effect", TestToolDefinition_ValidateRejectsUnknownSideEffect)
	t.Run("rejects write without filesystem scope", TestToolDefinition_ValidateRejectsWriteSideEffectWithoutScope)
	t.Run("accepts write with scope", TestToolDefinition_ValidateAcceptsWriteWithScope)
	t.Run("accepts none with empty scope", TestToolDefinition_ValidateAcceptsNoneWithEmptyScope)
}
