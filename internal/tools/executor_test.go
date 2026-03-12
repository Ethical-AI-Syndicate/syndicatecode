package tools

import (
	"context"
	"errors"
	"sync"
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

func TestExecutor_HandlerErrorIsReturned(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(ToolDefinition{
		Name:         "fails",
		Version:      "1",
		SideEffect:   SideEffectNone,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits: ExecutionLimits{
			TimeoutSeconds: 5,
			MaxOutputBytes: 1000,
		},
	})

	exec := NewExecutor(reg, nil)
	exec.RegisterHandler("fails", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return nil, errors.New("synthetic tool failure")
	})

	result, err := exec.Execute(context.Background(), ToolCall{ToolName: "fails", Input: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected handler error to be returned")
	}
	if result == nil || result.Success {
		t.Fatalf("expected failed result, got %+v", result)
	}
	if result.Error == "" {
		t.Fatal("expected error message to be recorded")
	}
}

func TestExecutor_InjectsSessionIDIntoToolInput(t *testing.T) {
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
		Limits: ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1000},
	})

	exec := NewExecutor(reg, nil)
	exec.RegisterHandler("echo", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		sessionID, _ := input["_session_id"].(string)
		return map[string]interface{}{"result": sessionID}, nil
	})

	result, err := exec.Execute(context.Background(), ToolCall{
		ToolName:  "echo",
		SessionID: "sess-123",
		Input: map[string]interface{}{
			"message": "hello",
		},
	})
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if got, _ := result.Output["result"].(string); got != "sess-123" {
		t.Fatalf("expected session id to be injected, got %q", got)
	}
}

type recordingCapture struct {
	mu      sync.Mutex
	calls   []string
	results []string
}

func (r *recordingCapture) BeforeExecute(_ context.Context, call ToolCall, _ ToolDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call.ToolName)
}
func (r *recordingCapture) AfterExecute(_ context.Context, _ ToolCall, _ ToolDefinition, _ *ToolResult, _ error, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, "recorded")
}

func TestExecutor_InvokesRecorder_Bead_l3d_15_4(t *testing.T) {
	cap := &recordingCapture{}
	reg := NewRegistry()
	if err := reg.Register(ToolDefinition{
		Name:         "echo",
		Version:      "1",
		SideEffect:   SideEffectNone,
		InputSchema:  map[string]FieldSchema{},
		OutputSchema: map[string]FieldSchema{},
		Limits:       ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}
	exec := NewExecutor(reg, nil)
	exec.SetRecorder(cap)
	exec.RegisterHandler("echo", func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		return input, nil
	})
	_, _ = exec.Execute(context.Background(), ToolCall{ID: "c1", ToolName: "echo", Input: map[string]interface{}{}})

	cap.mu.Lock()
	defer cap.mu.Unlock()
	if len(cap.calls) != 1 || cap.calls[0] != "echo" {
		t.Errorf("BeforeExecute not called, got %v", cap.calls)
	}
	if len(cap.results) != 1 {
		t.Errorf("AfterExecute not called, got %v", cap.results)
	}
}
