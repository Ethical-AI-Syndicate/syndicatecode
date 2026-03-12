package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrToolNotRegistered = errors.New("tool not registered")
var ErrOutputTooLarge = errors.New("output exceeds limit")
var ErrExecutionTimeout = errors.New("execution timeout")

type ToolHandler func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error)

// ExecutionRecorder is an optional hook called before and after tool execution.
// Wire a concrete implementation in the controlplane layer.
type ExecutionRecorder interface {
	BeforeExecute(ctx context.Context, call ToolCall, def ToolDefinition)
	AfterExecute(ctx context.Context, call ToolCall, def ToolDefinition, result *ToolResult, err error, duration time.Duration)
}

type Executor struct {
	registry *Registry
	handlers map[string]ToolHandler
	recorder ExecutionRecorder
	mu       sync.RWMutex
}

func (e *Executor) SetRecorder(r ExecutionRecorder) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.recorder = r
}

func NewExecutor(reg *Registry, handlers map[string]ToolHandler) *Executor {
	if handlers == nil {
		handlers = make(map[string]ToolHandler)
	}
	return &Executor{
		registry: reg,
		handlers: handlers,
	}
}

func (e *Executor) RegisterHandler(name string, handler ToolHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[name] = handler
}

func (e *Executor) Execute(ctx context.Context, call ToolCall) (*ToolResult, error) {
	tool, ok := e.registry.Get(call.ToolName)
	if !ok {
		return nil, ErrToolNotRegistered
	}

	result := &ToolResult{
		ID:      call.ID,
		Success: false,
	}

	start := time.Now()

	if e.recorder != nil {
		e.recorder.BeforeExecute(ctx, call, tool)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(tool.Limits.TimeoutSeconds)*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output map[string]interface{}
	var execErr error

	go func() {
		input := make(map[string]interface{}, len(call.Input)+1)
		for k, v := range call.Input {
			input[k] = v
		}
		if _, exists := input["_session_id"]; !exists && call.SessionID != "" {
			input["_session_id"] = call.SessionID
		}

		output, execErr = e.executeTool(ctx, call.ToolName, input)
		close(done)
	}()

	var returnErr error
	select {
	case <-done:
	case <-ctx.Done():
		result.Timeout = true
		result.Error = ErrExecutionTimeout.Error()
		result.Duration = time.Since(start).Milliseconds()
		returnErr = ErrExecutionTimeout
		if e.recorder != nil {
			e.recorder.AfterExecute(ctx, call, tool, result, returnErr, time.Since(start))
		}
		return result, returnErr
	}

	if execErr != nil {
		result.Error = execErr.Error()
		result.Duration = time.Since(start).Milliseconds()
		returnErr = execErr
		if e.recorder != nil {
			e.recorder.AfterExecute(ctx, call, tool, result, returnErr, time.Since(start))
		}
		return result, returnErr
	} else {
		result.Success = true
		result.Output = output

		if outputJSON, err := json.Marshal(output); err == nil && len(outputJSON) > tool.Limits.MaxOutputBytes {
			result.OutputTruncated = true
			result.Error = ErrOutputTooLarge.Error()
			result.Success = false
			returnErr = ErrOutputTooLarge
			if e.recorder != nil {
				e.recorder.AfterExecute(ctx, call, tool, result, returnErr, time.Since(start))
			}
			return result, returnErr
		}
	}

	result.Duration = time.Since(start).Milliseconds()
	if e.recorder != nil {
		e.recorder.AfterExecute(ctx, call, tool, result, nil, time.Since(start))
	}
	return result, nil
}

func (e *Executor) executeTool(ctx context.Context, name string, input map[string]interface{}) (map[string]interface{}, error) {
	e.mu.RLock()
	handler, ok := e.handlers[name]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no handler for tool: %s", name)
	}

	return handler(ctx, input)
}
