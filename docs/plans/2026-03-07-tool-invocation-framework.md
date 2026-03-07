# Tool Invocation Framework Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Build tool registry, execution engine, and policy integration that enables agents to safely interact with the codebase.

**Architecture:** Central tool registry with structured definitions, execution engine with constraints (timeout, output limits), policy evaluation before execution, and event logging for auditability.

**Tech Stack:** Go, SQLite (already in project)

---

### Task 1: Tool Definition Models

**Files:**
- Create: `internal/tools/models.go`
- Create: `internal/tools/models_test.go`

**Step 1: Write the failing test**

```go
package tools

import (
    "testing"
)

func TestToolDefinition_Validation(t *testing.T) {
    tool := ToolDefinition{
        Name:    "read_file",
        Version: "1",
        SideEffect: SideEffectRead,
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
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/tools/ -run TestToolDefinition`
Expected: FAIL - internal/tools/models.go does not exist

**Step 3: Write minimal implementation**

Create `internal/tools/models.go`:

```go
package tools

import (
    "errors"
    "fmt"
)

type SideEffect string

const (
    SideEffectNone     SideEffect = "none"
    SideEffectRead     SideEffect = "read"
    SideEffectWrite    SideEffect = "write"
    SideEffectExecute  SideEffect = "execute"
    SideEffectNetwork  SideEffect = "network"
)

type FieldSchema struct {
    Type        string `json:"type"`
    Description string `json:"description"`
    Required    bool   `json:"required"`
}

type ExecutionLimits struct {
    TimeoutSeconds int    `json:"timeout_seconds"`
    MaxOutputBytes int    `json:"max_output_bytes"`
    WorkingDir     string `json:"working_dir,omitempty"`
    AllowedPaths   []string `json:"allowed_paths,omitempty"`
}

type SecurityMetadata struct {
    NetworkAccess   bool   `json:"network_access"`
    FilesystemScope string  `json:"filesystem_scope"`
}

type ToolDefinition struct {
    Name               string                 `json:"name"`
    Version            string                 `json:"version"`
    Description        string                 `json:"description,omitempty"`
    SideEffect        SideEffect             `json:"side_effect"`
    ApprovalRequired  bool                   `json:"approval_required"`
    InputSchema       map[string]FieldSchema `json:"input_schema"`
    OutputSchema      map[string]FieldSchema `json:"output_schema"`
    Limits            ExecutionLimits         `json:"limits"`
    Security          SecurityMetadata        `json:"security,omitempty"`
}

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
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/tools/ -run TestToolDefinition`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tools/models.go internal/tools/models_test.go
git commit -m "feat(tools): add tool definition models and validation"
```

---

### Task 2: Tool Registry

**Files:**
- Modify: `internal/tools/models.go` (add ToolCall, ToolResult)
- Create: `internal/tools/registry.go`
- Create: `internal/tools/registry_test.go`

**Step 1: Write the failing test**

```go
package tools

import (
    "testing"
)

func TestRegistry_Register(t *testing.T) {
    reg := NewRegistry()
    
    tool := ToolDefinition{
        Name:    "test_tool",
        Version: "1",
        SideEffect: SideEffectRead,
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
        Name:    "duplicate",
        Version: "1",
        SideEffect: SideEffectNone,
        InputSchema: map[string]FieldSchema{},
        OutputSchema: map[string]FieldSchema{},
        Limits: ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
    }
    
    reg.Register(tool)
    err := reg.Register(tool)
    if err == nil {
        t.Error("expected error for duplicate registration")
    }
}

func TestRegistry_List(t *testing.T) {
    reg := NewRegistry()
    
    reg.Register(ToolDefinition{
        Name: "tool1", Version: "1", SideEffect: SideEffectRead,
        InputSchema: map[string]FieldSchema{}, OutputSchema: map[string]FieldSchema{},
        Limits: ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
    })
    reg.Register(ToolDefinition{
        Name: "tool2", Version: "1", SideEffect: SideEffectWrite,
        InputSchema: map[string]FieldSchema{}, OutputSchema: map[string]FieldSchema{},
        Limits: ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
    })
    
    tools := reg.List()
    if len(tools) != 2 {
        t.Errorf("got %d tools, want 2", len(tools))
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/tools/ -run TestRegistry`
Expected: FAIL - registry.go does not exist

**Step 3: Write minimal implementation**

Create `internal/tools/registry.go`:

```go
package tools

import (
    "errors"
    "sync"
)

var ErrToolAlreadyRegistered = errors.New("tool already registered")
var ErrToolNotFound = errors.New("tool not found")

type Registry struct {
    mu   sync.RWMutex
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
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/tools/ -run TestRegistry`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tools/registry.go internal/tools/registry_test.go
git commit -m "feat(tools): add tool registry with registration and lookup"
```

---

### Task 3: Tool Execution Engine

**Files:**
- Modify: `internal/tools/models.go` (add ToolCall, ToolResult types)
- Create: `internal/tools/executor.go`
- Create: `internal/tools/executor_test.go`

**Step 1: Write the failing test**

```go
package tools

import (
    "context"
    "testing"
    "time"
)

func TestExecutor_Execute(t *testing.T) {
    reg := NewRegistry()
    reg.Register(ToolDefinition{
        Name: "echo", Version: "1", SideEffect: SideEffectNone,
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
    reg.Register(ToolDefinition{
        Name: "slow", Version: "1", SideEffect: SideEffectExecute,
        InputSchema: map[string]FieldSchema{},
        OutputSchema: map[string]FieldSchema{},
        Limits: ExecutionLimits{
            TimeoutSeconds: 1,
            MaxOutputBytes: 1000,
        },
    })
    
    exec := NewExecutor(reg, nil)
    
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
    reg.Register(ToolDefinition{
        Name: "limited", Version: "1", SideEffect: SideEffectNone,
        InputSchema: map[string]FieldSchema{},
        OutputSchema: map[string]FieldSchema{},
        Limits: ExecutionLimits{
            TimeoutSeconds: 5,
            MaxOutputBytes: 10,
        },
    })
    
    exec := NewExecutor(reg, nil)
    
    call := ToolCall{
        ToolName: "limited",
        Input:    map[string]interface{}{},
    }
    
    result, err := exec.Execute(context.Background(), call)
    if err == nil {
        t.Error("expected output limit error")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/tools/ -run TestExecutor`
Expected: FAIL - executor.go does not exist, ToolCall/Result types missing

**Step 3: Write implementation**

First, add to models.go:

```go
type ToolCall struct {
    ToolName string                 `json:"tool_name"`
    Input    map[string]interface{} `json:"input"`
    ID       string                 `json:"id,omitempty"`
}

type ToolResult struct {
    ID        string                 `json:"id"`
    Success   bool                   `json:"success"`
    Output    map[string]interface{} `json:"output"`
    Error     string                 `json:"error,omitempty"`
    Duration  int64                  `json:"duration_ms"`
    Timeout   bool                  `json:"timeout,omitempty"`
    OutputTruncated bool             `json:"output_truncated,omitempty"`
}
```

Then create `internal/tools/executor.go`:

```go
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
        return result, nil
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
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/tools/ -run TestExecutor`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tools/models.go internal/tools/executor.go internal/tools/executor_test.go
git commit -m "feat(tools): add tool execution engine with timeout and output limits"
```

---

### Task 4: Policy Integration

**Files:**
- Create: `internal/policy/engine.go`
- Create: `internal/policy/engine_test.go`
- Modify: `internal/tools/executor.go` (integrate policy checks)

**Step 1: Write the failing test**

```go
package policy

import (
    "testing"
    
    "github.com/syndicatecode/syndicatecode/internal/tools"
)

func TestPolicyEngine_Evaluate(t *testing.T) {
    engine := NewEngine()
    
    engine.AddRule(Rule{
        Name:        "no_network",
        Description: "Block network access",
        Effect:      EffectDeny,
        Condition:   func(ctx *EvaluationContext) bool {
            return ctx.Tool.SideEffect == tools.SideEffectNetwork
        },
    })
    
    ctx := &EvaluationContext{
        Tool: tools.ToolDefinition{
            Name:       "http_get",
            SideEffect: tools.SideEffectNetwork,
        },
    }
    
    result := engine.Evaluate(ctx)
    if result.Allowed {
        t.Error("expected network access to be denied")
    }
}

func TestPolicyEngine_AllowRead(t *testing.T) {
    engine := NewEngine()
    
    ctx := &EvaluationContext{
        Tool: tools.ToolDefinition{
            Name:       "read_file",
            SideEffect: tools.SideEffectRead,
        },
    }
    
    result := engine.Evaluate(ctx)
    if !result.Allowed {
        t.Error("expected read access to be allowed")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/policy/ -run TestPolicyEngine`
Expected: FAIL - policy/engine.go does not exist

**Step 3: Write minimal implementation**

Create `internal/policy/engine.go`:

```go
package policy

import (
    "errors"
    
    "github.com/syndicatecode/syndicatecode/internal/tools"
)

type Effect string

const (
    EffectAllow Effect = "allow"
    EffectDeny  Effect = "deny"
)

type Rule struct {
    Name        string
    Description string
    Effect      Effect
    Condition   func(*EvaluationContext) bool
}

type EvaluationContext struct {
    Tool   tools.ToolDefinition
    Input  map[string]interface{}
    Session string
    User   string
}

type EvaluationResult struct {
    Allowed  bool
    DeniedBy []string
    Reason   string
}

type Engine struct {
    rules []Rule
}

func NewEngine() *Engine {
    return &Engine{
        rules: make([]Rule, 0),
    }
}

func (e *Engine) AddRule(rule Rule) {
    e.rules = append(e.rules, rule)
}

func (e *Engine) Evaluate(ctx *EvaluationContext) *EvaluationResult {
    result := &EvaluationResult{
        Allowed: true,
    }
    
    for _, rule := range e.rules {
        if rule.Condition(ctx) {
            if rule.Effect == EffectDeny {
                result.Allowed = false
                result.DeniedBy = append(result.DeniedBy, rule.Name)
                result.Reason = rule.Description
                break
            }
        }
    }
    
    return result
}

var ErrPolicyDenied = errors.New("policy denied")
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/policy/ -run TestPolicyEngine`
Expected: PASS

**Step 5: Integrate policy with executor**

Modify executor to check policy before execution:

```go
type Executor struct {
    registry  *Registry
    handlers  map[string]ToolHandler
    policy    *policy.Engine
    mu        sync.RWMutex
}

func NewExecutor(reg *Registry, handlers map[string]ToolHandler, policyEngine *policy.Engine) *Executor {
    if handlers == nil {
        handlers = make(map[string]ToolHandler)
    }
    return &Executor{
        registry: reg,
        handlers: handlers,
        policy:   policyEngine,
    }
}

func (e *Executor) Execute(ctx context.Context, call ToolCall) (*ToolResult, error) {
    tool, ok := e.registry.Get(call.ToolName)
    if !ok {
        return nil, ErrToolNotRegistered
    }
    
    // Policy check
    if e.policy != nil {
        policyCtx := &policy.EvaluationContext{
            Tool:   tool,
            Input:  call.Input,
        }
        result := e.policy.Evaluate(policyCtx)
        if !result.Allowed {
            return &ToolResult{
                ID:      call.ID,
                Success: false,
                Error:   "policy denied: " + result.Reason,
            }, nil
        }
    }
    
    // ... rest of execution
}
```

**Step 6: Commit**

```bash
git add internal/policy/engine.go internal/policy/engine_test.go internal/tools/executor.go
git commit -m "feat(tools): integrate policy engine with tool execution"
```

---

### Task 5: Built-in Tool Handlers

**Files:**
- Create: `internal/tools/builtin.go`
- Create: `internal/tools/builtin_test.go`

**Step 1: Write the failing test**

```go
package tools

import (
    "context"
    "os"
    "testing"
)

func TestBuiltin_Echo(t *testing.T) {
    handler := EchoHandler()
    
    result, err := handler(context.Background(), map[string]interface{}{
        "message": "test",
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if result["output"] != "test" {
        t.Errorf("got %v, want 'test'", result["output"])
    }
}

func TestBuiltin_ReadFile(t *testing.T) {
    handler := ReadFileHandler("/tmp")
    
    // Create temp file
    f, err := os.CreateTemp("/tmp", "test")
    if err != nil {
        t.Skip("cannot create temp file")
    }
    defer os.Remove(f.Name())
    f.WriteString("hello world")
    f.Close()
    
    result, err := handler(context.Background(), map[string]interface{}{
        "path": f.Name(),
    })
    
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    
    if result["content"] != "hello world" {
        t.Errorf("got %v, want 'hello world'", result["content"])
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/tools/ -run TestBuiltin`
Expected: FAIL - builtin.go does not exist

**Step 3: Write minimal implementation**

Create `internal/tools/builtin.go`:

```go
package tools

import (
    "context"
    "io/fs"
    "os"
    "path/filepath"
)

func EchoHandler() ToolHandler {
    return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
        msg, _ := input["message"].(string)
        return map[string]interface{}{
            "output": msg,
        }, nil
    }
}

func ReadFileHandler(allowedDir string) ToolHandler {
    return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
        path, ok := input["path"].(string)
        if !ok {
            return nil, ErrInvalidInput
        }
        
        absPath, err := filepath.Abs(path)
        if err != nil {
            return nil, err
        }
        
        if !isPathInDir(absPath, allowedDir) {
            return nil, ErrPathNotAllowed
        }
        
        content, err := os.ReadFile(absPath)
        if err != nil {
            return nil, err
        }
        
        info, _ := os.Stat(absPath)
        
        return map[string]interface{}{
            "content":    string(content),
            "size":       info.Size(),
            "path":       absPath,
            "mod_time":   info.ModTime().Unix(),
        }, nil
    }
}

func isPathInDir(path, dir string) bool {
    absPath, _ := filepath.Abs(path)
    absDir, _ := filepath.Abs(dir)
    return absPath == absDir || len(absPath) > len(absDir) && absPath[:len(absDir)+1] == absDir+"/"
}

var ErrInvalidInput = &ToolError{Message: "invalid input"}
var ErrPathNotAllowed = &ToolError{Message: "path not allowed"}

type ToolError struct {
    Message string
}

func (e *ToolError) Error() string {
    return e.Message
}

func WriteFileHandler(allowedDir string) ToolHandler {
    return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
        path, ok := input["path"].(string)
        if !ok {
            return nil, ErrInvalidInput
        }
        
        content, ok := input["content"].(string)
        if !ok {
            return nil, ErrInvalidInput
        }
        
        absPath, err := filepath.Abs(path)
        if err != nil {
            return nil, err
        }
        
        if !isPathInDir(absPath, allowedDir) {
            return nil, ErrPathNotAllowed
        }
        
        err = os.WriteFile(absPath, []byte(content), fs.FileMode(0644))
        if err != nil {
            return nil, err
        }
        
        return map[string]interface{}{
            "path":        absPath,
            "bytes_written": len(content),
        }, nil
    }
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./internal/tools/ -run TestBuiltin`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tools/builtin.go internal/tools/builtin_test.go
git commit -m "feat(tools): add built-in tool handlers (echo, read_file, write_file)"
```

---

### Task 6: Integration with Control Plane

**Files:**
- Modify: `internal/controlplane/server.go` (add tool endpoints)
- Create: `internal/tools/api.go`

**Step 1: Write the failing test**

```go
package controlplane

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/syndicatecode/syndicatecode/internal/tools"
)

func TestToolEndpoints_List(t *testing.T) {
    reg := tools.NewRegistry()
    reg.Register(tools.ToolDefinition{
        Name: "test", Version: "1", SideEffect: tools.SideEffectNone,
        InputSchema: map[string]tools.FieldSchema{},
        OutputSchema: map[string]tools.FieldSchema{},
        Limits: tools.ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
    })
    
    server := NewServer(WithToolRegistry(reg))
    
    req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
    w := httptest.NewRecorder()
    
    server.router.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("got %d, want %d", w.Code, http.StatusOK)
    }
    
    var response map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &response)
    
    if response["tools"] == nil {
        t.Error("expected tools in response")
    }
}

func TestToolEndpoints_Execute(t *testing.T) {
    reg := tools.NewRegistry()
    reg.Register(tools.ToolDefinition{
        Name: "echo", Version: "1", SideEffect: tools.SideEffectNone,
        InputSchema: map[string]tools.FieldSchema{
            "message": {Type: "string"},
        },
        OutputSchema: map[string]tools.FieldSchema{
            "output": {Type: "string"},
        },
        Limits: tools.ExecutionLimits{TimeoutSeconds: 30, MaxOutputBytes: 1000},
    })
    
    exec := tools.NewExecutor(reg, nil, nil)
    exec.RegisterHandler("echo", tools.EchoHandler())
    
    server := NewServer(WithToolRegistry(reg), WithToolExecutor(exec))
    
    body := `{"tool_name": "echo", "input": {"message": "hello"}}`
    req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", 
        strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    
    server.router.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("got %d, want %d", w.Code, http.StatusOK)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./internal/controlplane/ -run TestToolEndpoints`
Expected: FAIL - API endpoints don't exist

**Step 3: Write implementation**

Create `internal/tools/api.go`:

```go
package tools

import (
    "encoding/json"
    "net/http"
)

type ListToolsResponse struct {
    Tools []ToolDefinition `json:"tools"`
}

type ExecuteRequest struct {
    ToolName string                 `json:"tool_name"`
    Input    map[string]interface{} `json:"input"`
}

type ExecuteResponse struct {
    Result *ToolResult `json:"result"`
    Error  string      `json:"error,omitempty"`
}

func (r *Registry) HandleList(w http.ResponseWriter, req *http.Request) {
    tools := r.List()
    response := ListToolsResponse{
        Tools: tools,
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

func (e *Executor) HandleExecute(w http.ResponseWriter, req *http.Request) {
    var executeReq ExecuteRequest
    if err := json.NewDecoder(req.Body).Decode(&executeReq); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    call := ToolCall{
        ToolName: executeReq.ToolName,
        Input:    executeReq.Input,
    }
    
    result, err := e.Execute(req.Context(), call)
    if err != nil {
        response := ExecuteResponse{
            Error: err.Error(),
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
        return
    }
    
    response := ExecuteResponse{
        Result: result,
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}
```

**Step 4: Commit**

```bash
git add internal/tools/api.go
git commit -m "feat(tools): add HTTP handlers for tool registry and execution"
```

---

## Summary

After completing all tasks, the Tool Invocation Framework will provide:

1. **Tool Definition Models** - Structured tool definitions with validation
2. **Tool Registry** - Central registration and lookup of available tools
3. **Execution Engine** - Safe execution with timeout, output limits, and policy checks
4. **Policy Integration** - Deny rules based on tool characteristics
5. **Built-in Handlers** - Echo, Read, Write tools with directory restrictions
6. **HTTP API** - REST endpoints for tool listing and execution

This framework enables the agent runtime to safely interact with the codebase while maintaining auditability and policy enforcement.
