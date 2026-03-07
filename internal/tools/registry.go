package tools

import (
	"errors"
	"sync"
)

var ErrToolAlreadyRegistered = errors.New("tool already registered")
var ErrToolNotFound = errors.New("tool not found")

type Registry struct {
	mu    sync.RWMutex
	tools map[string]ToolDefinition
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]ToolDefinition),
	}
}

func (r *Registry) Register(tool ToolDefinition) error {
	if err := tool.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[tool.Name]; exists {
		return ErrToolAlreadyRegistered
	}

	r.tools[tool.Name] = tool
	return nil
}

func (r *Registry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}
