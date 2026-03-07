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
