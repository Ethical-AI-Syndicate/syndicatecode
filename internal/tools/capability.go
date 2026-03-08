package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrInvalidToolCapability = errors.New("invalid tool capability")
	ErrToolNotRegistered     = errors.New("tool not registered")
)

type SideEffectClass string

const (
	SideEffectNone    SideEffectClass = "none"
	SideEffectRead    SideEffectClass = "read"
	SideEffectWrite   SideEffectClass = "write"
	SideEffectExecute SideEffectClass = "execute"
	SideEffectNetwork SideEffectClass = "network"
	SideEffectShell   SideEffectClass = "shell"
)

type IsolationLevel string

const (
	IsolationLevel0 IsolationLevel = "l0"
	IsolationLevel1 IsolationLevel = "l1"
	IsolationLevel2 IsolationLevel = "l2"
)

type FilesystemScope string

const (
	FilesystemScopeRepoOnly      FilesystemScope = "repo_only"
	FilesystemScopeRepoPlusTemp  FilesystemScope = "repo_plus_temp"
	FilesystemScopeAllowlistPath FilesystemScope = "allowlisted_paths"
)

type NetworkClass string

const (
	NetworkNone      NetworkClass = "none"
	NetworkAllowlist NetworkClass = "allowlisted"
	NetworkToolOnly  NetworkClass = "tool_specific"
	NetworkOpen      NetworkClass = "open"
)

type ExecutionLimits struct {
	TimeoutSeconds int
	MaxOutputBytes int
}

type ToolCapability struct {
	Name            string
	Version         int
	SideEffectClass SideEffectClass
	IsolationLevel  IsolationLevel
	FilesystemScope FilesystemScope
	NetworkClass    NetworkClass
	Limits          ExecutionLimits
}

func (c ToolCapability) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidToolCapability)
	}
	if c.Version <= 0 {
		return fmt.Errorf("%w: version must be positive", ErrInvalidToolCapability)
	}
	if !isValidSideEffect(c.SideEffectClass) {
		return fmt.Errorf("%w: invalid side effect class %q", ErrInvalidToolCapability, c.SideEffectClass)
	}
	if !isValidIsolationLevel(c.IsolationLevel) {
		return fmt.Errorf("%w: invalid isolation level %q", ErrInvalidToolCapability, c.IsolationLevel)
	}
	if !isValidFilesystemScope(c.FilesystemScope) {
		return fmt.Errorf("%w: invalid filesystem scope %q", ErrInvalidToolCapability, c.FilesystemScope)
	}
	if !isValidNetworkClass(c.NetworkClass) {
		return fmt.Errorf("%w: invalid network class %q", ErrInvalidToolCapability, c.NetworkClass)
	}
	if c.Limits.TimeoutSeconds <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidToolCapability)
	}
	if c.Limits.MaxOutputBytes <= 0 {
		return fmt.Errorf("%w: max output bytes must be positive", ErrInvalidToolCapability)
	}

	return nil
}

func isValidSideEffect(v SideEffectClass) bool {
	switch v {
	case SideEffectNone, SideEffectRead, SideEffectWrite, SideEffectExecute, SideEffectNetwork, SideEffectShell:
		return true
	default:
		return false
	}
}

func isValidIsolationLevel(v IsolationLevel) bool {
	switch v {
	case IsolationLevel0, IsolationLevel1, IsolationLevel2:
		return true
	default:
		return false
	}
}

func isValidFilesystemScope(v FilesystemScope) bool {
	switch v {
	case FilesystemScopeRepoOnly, FilesystemScopeRepoPlusTemp, FilesystemScopeAllowlistPath:
		return true
	default:
		return false
	}
}

func isValidNetworkClass(v NetworkClass) bool {
	switch v {
	case NetworkNone, NetworkAllowlist, NetworkToolOnly, NetworkOpen:
		return true
	default:
		return false
	}
}

type Registry struct {
	mu           sync.RWMutex
	capabilities map[string]ToolCapability
}

func NewRegistry() *Registry {
	return &Registry{capabilities: make(map[string]ToolCapability)}
}

func (r *Registry) Register(capability ToolCapability) error {
	if err := capability.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[capability.Name] = capability

	return nil
}

func (r *Registry) RegisterUnchecked(capability ToolCapability) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[capability.Name] = capability

	return nil
}

func (r *Registry) Get(name string) (ToolCapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	capability, ok := r.capabilities[name]
	return capability, ok
}

type Runner interface {
	Run(ctx context.Context, capability ToolCapability, args map[string]any) (map[string]any, error)
}

type StubRunner struct{}

func (s StubRunner) Run(_ context.Context, _ ToolCapability, _ map[string]any) (map[string]any, error) {
	return map[string]any{"status": "ok"}, nil
}

type Executor struct {
	registry *Registry
	runner   Runner
}

func NewExecutor(registry *Registry, runner Runner) *Executor {
	return &Executor{
		registry: registry,
		runner:   runner,
	}
}

func (e *Executor) Execute(ctx context.Context, toolName string, args map[string]any) (map[string]any, error) {
	capability, ok := e.registry.Get(toolName)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotRegistered, toolName)
	}
	if err := capability.Validate(); err != nil {
		return nil, err
	}

	result, err := e.runner.Run(ctx, capability, args)
	if err != nil {
		return nil, fmt.Errorf("failed to execute tool %s: %w", toolName, err)
	}

	return result, nil
}
