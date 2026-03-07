package tools

import (
	"context"
	"testing"
	"time"
)

func TestExecutor_Execute(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(ToolDefinition{
		Name:       "echo",
		Version:    "1",
		SideEffect: SideEffectNone,
		InputSchema: map[string]FieldSchema{
			"message": {Type: "string"},
		},
		OutputSchema: map[string]FieldSchema{
			"result": {Type: "string"},
		},
		Limits: ExecutionLimits{
			TimeoutSeconds: 5,
			MaxOutputBytes: 1000,
		},
	})

	exec := NewExecutor(reg, nil)
	exec.RegisterHandler("echo", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		msg, _ := input["message"].(string)
		return map[string]interface{}{"result": msg}, nil
	})

	call := ToolCall{
		ToolName: "echo",
		Input: map[string]interface{}{
			"message": "hello",
		},
	}

	result, err := exec.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if !result.Success {
		t.Error("expected success")
	}
}

func TestExecutor_Timeout(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(ToolDefinition{
		Name:         "slow",
		Version:      "1",
		SideEffect:   SideEffectExecute,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits: ExecutionLimits{
			TimeoutSeconds: 1,
			MaxOutputBytes: 1000,
		},
	})

	exec := NewExecutor(reg, nil)
	exec.RegisterHandler("slow", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		time.Sleep(5 * time.Second)
		return nil, nil
	})

	call := ToolCall{
		ToolName: "slow",
		Input:    map[string]interface{}{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := exec.Execute(ctx, call)
	if err == nil {
		t.Error("expected timeout error")
	}
	if result != nil && !result.Timeout {
		t.Error("expected timeout flag")
	}
}

func TestExecutor_OutputLimit(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(ToolDefinition{
		Name:         "limited",
		Version:      "1",
		SideEffect:   SideEffectNone,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits: ExecutionLimits{
			TimeoutSeconds: 5,
			MaxOutputBytes: 10,
		},
	})

	exec := NewExecutor(reg, nil)
	exec.RegisterHandler("limited", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return map[string]interface{}{
			"output": "this is a very long output that exceeds the limit",
		}, nil
	})

	call := ToolCall{
		ToolName: "limited",
		Input:    map[string]interface{}{},
	}

	result, err := exec.Execute(context.Background(), call)
	if err == nil {
		t.Error("expected output limit error")
	}
	if result != nil && !result.OutputTruncated {
		t.Error("expected output truncated flag")
	}
}
