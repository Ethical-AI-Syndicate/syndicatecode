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

type Executor struct {
	registry *Registry
	handlers map[string]ToolHandler
	mu       sync.RWMutex
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

	ctx, cancel := context.WithTimeout(ctx, time.Duration(tool.Limits.TimeoutSeconds)*time.Second)
	defer cancel()

	done := make(chan struct{})
	var output map[string]interface{}
	var execErr error

	go func() {
		output, execErr = e.executeTool(ctx, call.ToolName, call.Input)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		result.Timeout = true
		result.Error = ErrExecutionTimeout.Error()
		result.Duration = time.Since(start).Milliseconds()
		return result, ErrExecutionTimeout
	}

	if execErr != nil {
		result.Error = execErr.Error()
	} else {
		result.Success = true
		result.Output = output

		if outputJSON, err := json.Marshal(output); err == nil && len(outputJSON) > tool.Limits.MaxOutputBytes {
			result.OutputTruncated = true
			result.Error = ErrOutputTooLarge.Error()
			result.Success = false
			return result, ErrOutputTooLarge
		}
	}

	result.Duration = time.Since(start).Milliseconds()
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
