package tools

import (
	"errors"
	"fmt"
)

type SideEffect string

const (
	SideEffectNone    SideEffect = "none"
	SideEffectRead    SideEffect = "read"
	SideEffectWrite   SideEffect = "write"
	SideEffectExecute SideEffect = "execute"
	SideEffectNetwork SideEffect = "network"
)

type FieldSchema struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type ExecutionLimits struct {
	TimeoutSeconds int      `json:"timeout_seconds"`
	MaxOutputBytes int      `json:"max_output_bytes"`
	WorkingDir     string   `json:"working_dir,omitempty"`
	AllowedPaths   []string `json:"allowed_paths,omitempty"`
}

type SecurityMetadata struct {
	NetworkAccess   bool   `json:"network_access"`
	FilesystemScope string `json:"filesystem_scope"`
}

type ToolDefinition struct {
	Name             string                 `json:"name"`
	Version          string                 `json:"version"`
	Description      string                 `json:"description,omitempty"`
	Source           string                 `json:"source,omitempty"`
	TrustLevel       string                 `json:"trust_level,omitempty"`
	SideEffect       SideEffect             `json:"side_effect"`
	ApprovalRequired bool                   `json:"approval_required"`
	InputSchema      map[string]FieldSchema `json:"input_schema"`
	OutputSchema     map[string]FieldSchema `json:"output_schema"`
	Limits           ExecutionLimits        `json:"limits"`
	Security         SecurityMetadata       `json:"security,omitempty"`
}

const (
	ToolSourceCore   = "core"
	ToolSourcePlugin = "plugin"
)

func (t *ToolDefinition) Validate() error {
	if t.Name == "" {
		return errors.New("tool name is required")
	}
	if t.Version == "" {
		return errors.New("tool version is required")
	}
	if t.InputSchema == nil {
		return errors.New("input schema is required")
	}
	if t.OutputSchema == nil {
		return errors.New("output schema is required")
	}
	if t.Limits.TimeoutSeconds <= 0 {
		return fmt.Errorf("invalid timeout: %d", t.Limits.TimeoutSeconds)
	}
	if t.Limits.MaxOutputBytes <= 0 {
		return fmt.Errorf("invalid max output: %d", t.Limits.MaxOutputBytes)
	}
	return nil
}

type ToolCall struct {
	ToolName  string                 `json:"tool_name"`
	SessionID string                 `json:"session_id,omitempty"`
	Input     map[string]interface{} `json:"input"`
	ID        string                 `json:"id,omitempty"`
}

type ToolResult struct {
	ID              string                 `json:"id"`
	Success         bool                   `json:"success"`
	Output          map[string]interface{} `json:"output"`
	Error           string                 `json:"error,omitempty"`
	Duration        int64                  `json:"duration_ms"`
	Timeout         bool                   `json:"timeout,omitempty"`
	OutputTruncated bool                   `json:"output_truncated,omitempty"`
}
