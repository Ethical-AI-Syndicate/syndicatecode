# Gap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close all 11 architectural gaps identified in the 2026-03-11 production audit,
delivering a fully-wired control plane with agent runtime, complete audit trail, and
streaming model responses.

**Architecture:** Three sequentially-gated phases — persistence completeness first
(event types + DB writes + budget wiring), then model provider abstraction and ReAct
agent loop with streaming, then trust package extraction and UX gaps. Each phase
produces independently verifiable, bead-evidenced increments.

**Tech Stack:** Go 1.25, SQLite (WAL), WebSocket (nhooyr.io/websocket), Anthropic Go SDK,
OpenAI Go SDK, Vercel AI SDK interface patterns in Go.

---

## Pre-Flight

Before any task, register beads in `bd` for each phase and record their IDs.
Every non-merge commit **must** include `[l3d.X.Y]` in the subject.
Every bead-tagged test function **must** use the `_Bead_l3d_X_Y` suffix.

Quality gate after every commit:
```bash
make format-check lint test build
```

Module path throughout: `gitlab.mikeholownych.com/ai-syndicate/syndicatecode`

---

## Phase 1 — Persistence Completeness

Closes: GAP 2 (three dead DB tables), GAP 3 (four missing event types), GAP 4 (BudgetAllocator unwired).

---

### Task 1: Event Type Constants

**Files:**
- Modify: `internal/audit/event_types.go`
- Test: `internal/audit/event_store_test.go`

**Step 1: Write the failing test**

Add to `internal/audit/event_store_test.go`:

```go
func TestEventTypeConstants_Bead_l3d_X_1(t *testing.T) {
    // Verify all spec-required event types exist as non-empty constants
    types := []string{
        EventModelInvoked,
        EventToolInvoked,
        EventToolResult,
        EventFileMutation,
    }
    for _, et := range types {
        if et == "" {
            t.Errorf("event type constant is empty")
        }
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/audit/... -run TestEventTypeConstants -v
```
Expected: `undefined: EventModelInvoked`

**Step 3: Add constants to `internal/audit/event_types.go`**

```go
EventModelInvoked = "model_invocation"
EventToolInvoked  = "tool_invocation"
EventToolResult   = "tool_result"
EventFileMutation = "file_mutation"
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/audit/... -run TestEventTypeConstants -v
```
Expected: PASS

**Step 5: Commit**
```bash
git add internal/audit/event_types.go internal/audit/event_store_test.go
git commit -m "feat(audit): add missing event type constants [l3d.X.1]"
```

---

### Task 2: EventStore Write Methods — Tool and Model Invocations

**Files:**
- Modify: `internal/audit/event_store.go`
- Test: `internal/audit/event_store_test.go`

**Step 1: Write failing tests**

Add to `internal/audit/event_store_test.go`:

```go
func TestEventStore_RecordToolInvocation_Bead_l3d_X_1(t *testing.T) {
    store := newTestStore(t)
    rec := ToolInvocationRecord{
        ID:         "inv-1",
        SessionID:  "sess-1",
        TurnID:     "turn-1",
        ToolName:   "read_file",
        Success:    true,
        DurationMS: 42,
        CreatedAt:  time.Now().UTC(),
    }
    if err := store.RecordToolInvocation(context.Background(), rec); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}

func TestEventStore_RecordModelInvocation_Bead_l3d_X_1(t *testing.T) {
    store := newTestStore(t)
    rec := ModelInvocationRecord{
        ID:        "mod-1",
        SessionID: "sess-1",
        TurnID:    "turn-1",
        Provider:  "anthropic",
        Model:     "claude-sonnet-4-6",
        CreatedAt: time.Now().UTC(),
    }
    if err := store.RecordModelInvocation(context.Background(), rec); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/audit/... -run TestEventStore_Record -v
```
Expected: `undefined: ToolInvocationRecord`

**Step 3: Add record types and write methods to `internal/audit/event_store.go`**

Add the record types (place near the existing `ArtifactRecord` type):

```go
type ToolInvocationRecord struct {
    ID         string
    SessionID  string
    TurnID     string
    ApprovalID string
    ToolName   string
    Success    bool
    DurationMS int64
    OutputRef  string
    CreatedAt  time.Time
}

type ModelInvocationRecord struct {
    ID            string
    SessionID     string
    TurnID        string
    Provider      string
    Model         string
    RoutingPolicy string
    PromptRef     string
    ResponseRef   string
    CreatedAt     time.Time
}
```

Add the write methods:

```go
func (s *EventStore) RecordToolInvocation(ctx context.Context, r ToolInvocationRecord) error {
    success := 0
    if r.Success {
        success = 1
    }
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO tool_invocations
         (id, session_id, turn_id, approval_id, tool_name, success, duration_ms, output_ref, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        r.ID, r.SessionID, r.TurnID, r.ApprovalID,
        r.ToolName, success, r.DurationMS, r.OutputRef,
        r.CreatedAt.UTC().Format(time.RFC3339Nano),
    )
    return err
}

func (s *EventStore) RecordModelInvocation(ctx context.Context, r ModelInvocationRecord) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO model_invocations
         (id, session_id, turn_id, provider, model, routing_policy, prompt_ref, response_ref, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        r.ID, r.SessionID, r.TurnID, r.Provider, r.Model,
        r.RoutingPolicy, r.PromptRef, r.ResponseRef,
        r.CreatedAt.UTC().Format(time.RFC3339Nano),
    )
    return err
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/audit/... -run TestEventStore_Record -v
```

**Step 5: Commit**
```bash
git add internal/audit/event_store.go internal/audit/event_store_test.go
git commit -m "feat(audit): add RecordToolInvocation and RecordModelInvocation [l3d.X.1]"
```

---

### Task 3: EventStore Write Method — File Mutations

**Files:**
- Modify: `internal/audit/event_store.go`
- Test: `internal/audit/event_store_test.go`

**Step 1: Write failing test**

```go
func TestEventStore_RecordFileMutation_Bead_l3d_X_1(t *testing.T) {
    store := newTestStore(t)
    rec := FileMutationRecord{
        ID:           "mut-1",
        SessionID:    "sess-1",
        TurnID:       "turn-1",
        PatchID:      "patch-1",
        Path:         "internal/foo/bar.go",
        MutationType: "update",
        BeforeHash:   "abc123",
        AfterHash:    "def456",
        AppliedAt:    time.Now().UTC(),
    }
    if err := store.RecordFileMutation(context.Background(), rec); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/audit/... -run TestEventStore_RecordFile -v
```

**Step 3: Add record type and write method**

```go
type FileMutationRecord struct {
    ID           string
    SessionID    string
    TurnID       string
    PatchID      string
    Path         string
    MutationType string
    BeforeHash   string
    AfterHash    string
    AppliedAt    time.Time
}

func (s *EventStore) RecordFileMutation(ctx context.Context, r FileMutationRecord) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO file_mutations
         (id, session_id, turn_id, patch_id, path, mutation_type, before_hash, after_hash, applied_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        r.ID, r.SessionID, r.TurnID, r.PatchID,
        r.Path, r.MutationType, r.BeforeHash, r.AfterHash,
        r.AppliedAt.UTC().Format(time.RFC3339Nano),
    )
    return err
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/audit/... -run TestEventStore_RecordFile -v
```

**Step 5: Commit**
```bash
git add internal/audit/event_store.go internal/audit/event_store_test.go
git commit -m "feat(audit): add RecordFileMutation [l3d.X.1]"
```

---

### Task 4: Wire Recording into Executor and Patch Handler

**Files:**
- Modify: `internal/tools/executor.go`
- Modify: `internal/tools/executor_test.go`
- Modify: `internal/controlplane/server.go`

The `Executor` must not import `internal/audit` (package boundary). Use an interface.

**Step 1: Write failing test in `internal/tools/executor_test.go`**

```go
type recordingCapture struct {
    calls   []string
    results []string
}

func (r *recordingCapture) BeforeExecute(_ context.Context, call ToolCall, _ ToolDefinition) {
    r.calls = append(r.calls, call.ToolName)
}
func (r *recordingCapture) AfterExecute(_ context.Context, _ ToolCall, _ ToolDefinition, _ *ToolResult, _ error, _ time.Duration) {
    r.results = append(r.results, "recorded")
}

func TestExecutor_InvokesRecorder_Bead_l3d_X_1(t *testing.T) {
    cap := &recordingCapture{}
    reg := NewRegistry()
    // register a minimal tool definition
    reg.Register(ToolDefinition{
        Name: "echo",
        Limits: ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
    })
    exec := NewExecutor(reg, nil)
    exec.SetRecorder(cap)
    exec.RegisterHandler("echo", func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
        return input, nil
    })
    _, _ = exec.Execute(context.Background(), ToolCall{ID: "c1", ToolName: "echo", Input: map[string]interface{}{}})
    if len(cap.calls) != 1 || cap.calls[0] != "echo" {
        t.Errorf("recorder not called before execute, got %v", cap.calls)
    }
    if len(cap.results) != 1 {
        t.Errorf("recorder not called after execute, got %v", cap.results)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/tools/... -run TestExecutor_InvokesRecorder -v
```

**Step 3: Add `ExecutionRecorder` interface and `SetRecorder` to `internal/tools/executor.go`**

Add after the existing `ToolHandler` type:

```go
// ExecutionRecorder is an optional hook for recording tool invocations.
// Implement in the controlplane layer; pass nil to disable.
type ExecutionRecorder interface {
    BeforeExecute(ctx context.Context, call ToolCall, def ToolDefinition)
    AfterExecute(ctx context.Context, call ToolCall, def ToolDefinition, result *ToolResult, err error, duration time.Duration)
}
```

Add `recorder ExecutionRecorder` field to `Executor` struct and a setter:

```go
func (e *Executor) SetRecorder(r ExecutionRecorder) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.recorder = r
}
```

At the top of `Execute`, capture `tool` def for the recorder call. Before the goroutine, call `e.recorder.BeforeExecute`. After the goroutine resolves, call `e.recorder.AfterExecute`. Guard both with a nil check:

```go
if e.recorder != nil {
    e.recorder.BeforeExecute(ctx, call, tool)
}
// ... existing execution goroutine ...
if e.recorder != nil {
    e.recorder.AfterExecute(ctx, call, tool, result, execErr, time.Since(start))
}
```

**Step 4: Implement the concrete recorder in `internal/controlplane/server.go`**

Add a private type below the `Server` struct definition:

```go
type auditExecutionRecorder struct {
    store     *audit.EventStore
    sessionID func(ctx context.Context) string // extract from request context
}

func (r *auditExecutionRecorder) BeforeExecute(ctx context.Context, call tools.ToolCall, def tools.ToolDefinition) {
    _ = r.store.Append(ctx, audit.Event{
        ID:        uuid.New().String(),
        SessionID: call.SessionID,
        Timestamp: time.Now().UTC(),
        EventType: audit.EventToolInvoked,
        Actor:     requestmeta.Actor(ctx),
        Payload:   map[string]string{"tool_name": call.ToolName, "tool_call_id": call.ID},
    })
}

func (r *auditExecutionRecorder) AfterExecute(ctx context.Context, call tools.ToolCall, def tools.ToolDefinition, result *tools.ToolResult, err error, duration time.Duration) {
    success := err == nil && result != nil && result.Success
    recErr := r.store.RecordToolInvocation(ctx, audit.ToolInvocationRecord{
        ID:         uuid.New().String(),
        SessionID:  call.SessionID,
        ToolName:   call.ToolName,
        Success:    success,
        DurationMS: duration.Milliseconds(),
        CreatedAt:  time.Now().UTC(),
    })
    if recErr != nil {
        log.Printf("failed to record tool invocation: %v", recErr)
    }
    _ = r.store.Append(ctx, audit.Event{
        ID:        uuid.New().String(),
        SessionID: call.SessionID,
        Timestamp: time.Now().UTC(),
        EventType: audit.EventToolResult,
        Actor:     requestmeta.Actor(ctx),
        Payload:   map[string]string{"tool_name": call.ToolName, "success": fmt.Sprintf("%v", success)},
    })
}
```

In the server's `NewServer` (or wherever `toolExecutor` is wired), add:
```go
s.toolExecutor.SetRecorder(&auditExecutionRecorder{store: s.eventStore})
```

**Step 5: Wire file mutation recording in `internal/tools/patch_handler.go`**

The `ApplyPatchHandler` currently returns a `ToolHandler`. It needs access to the event store to emit file mutations. Add an optional `MutationRecorder` function type:

```go
// MutationRecorder is called for each successfully applied file operation.
type MutationRecorder func(ctx context.Context, rec audit.FileMutationRecord)
```

Modify `ApplyPatchHandler` signature to accept an optional recorder:

```go
func ApplyPatchHandler(engine *patch.Engine, recorder MutationRecorder) ToolHandler {
```

After each successful operation apply, call:
```go
if recorder != nil {
    recorder(ctx, audit.FileMutationRecord{
        ID:           uuid.New().String(),
        Path:         op.TargetPath,
        MutationType: string(op.Type),
        BeforeHash:   op.PreimageHash,
        AfterHash:    op.ExpectedPostimageHash,
        AppliedAt:    time.Now().UTC(),
    })
}
```

Update the registration in `server.go`:
```go
executor.RegisterHandler("apply_patch", tools.ApplyPatchHandler(
    patch.NewEngine(repoRoot),
    func(ctx context.Context, rec audit.FileMutationRecord) {
        rec.SessionID = // extract from ctx if available
        if err := s.eventStore.RecordFileMutation(ctx, rec); err != nil {
            log.Printf("failed to record file mutation: %v", err)
        }
        _ = s.eventStore.Append(ctx, audit.Event{
            ID:        uuid.New().String(),
            Timestamp: time.Now().UTC(),
            EventType: audit.EventFileMutation,
            Payload:   map[string]string{"path": rec.Path, "type": rec.MutationType},
        })
    },
))
```

**Step 6: Run full test suite**
```bash
make format-check lint test build
```

**Step 7: Commit**
```bash
git add internal/tools/ internal/controlplane/server.go
git commit -m "feat(tools,controlplane): wire execution and mutation recording [l3d.X.1]"
```

---

### Task 5: Wire BudgetAllocator into ContextAssembler

**Files:**
- Modify: `internal/context/context.go`
- Test: `internal/context/context_test.go`

The `ContextAssembler.AddFragment` uses a simple token sum check. Add a new method
`AssembleFromRanked` that uses `BudgetAllocator.AllocateFragments` for category-aware
allocation. The agent runtime (Phase 2) will call this.

**Step 1: Write failing test in `internal/context/context_test.go`**

```go
func TestContextAssembler_AssembleFromRanked_Bead_l3d_X_1(t *testing.T) {
    budget := DefaultCategoryBudget(4000)
    assembler := NewContextAssemblerWithPolicy(budget.Total, nil)

    ranked := []RankedFragment{
        {Fragment: ContextFragment{SourceType: "instruction", TokenCount: 100}, Rank: 10},
        {Fragment: ContextFragment{SourceType: "file", TokenCount: 200}, Rank: 5},
        {Fragment: ContextFragment{SourceType: "file", TokenCount: 5000}, Rank: 3}, // over budget
    }

    fragments := assembler.AssembleFromRanked(ranked, budget)

    // Should include high-rank fragments that fit
    if len(fragments) == 0 {
        t.Fatal("expected at least one fragment to be allocated")
    }
    // The over-budget fragment should not crowd out the others
    totalTokens := 0
    for _, f := range fragments {
        totalTokens += f.TokenCount
    }
    if totalTokens > budget.Total {
        t.Errorf("total tokens %d exceeds budget %d", totalTokens, budget.Total)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/context/... -run TestContextAssembler_AssembleFromRanked -v
```

**Step 3: Add `AssembleFromRanked` method to `ContextAssembler`**

In `internal/context/context.go`, add after `AddFragment`:

```go
// AssembleFromRanked uses BudgetAllocator to select fragments within category budgets.
// Call this instead of AddFragment when ranked fragments are available.
func (a *ContextAssembler) AssembleFromRanked(ranked []RankedFragment, budget CategoryBudget) []ContextFragment {
    allocator := NewBudgetAllocator(budget)
    allocated, _ := allocator.AllocateFragments(ranked)
    a.fragments = make([]*ContextFragment, 0, len(allocated))
    for i := range allocated {
        a.fragments = append(a.fragments, &allocated[i])
    }
    return allocated
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/context/... -run TestContextAssembler_AssembleFromRanked -v
```

**Step 5: Run full gate**
```bash
make format-check lint test build
```

**Step 6: Commit**
```bash
git add internal/context/context.go internal/context/context_test.go
git commit -m "feat(context): wire BudgetAllocator into ContextAssembler.AssembleFromRanked [l3d.X.1]"
```

---

## Phase 2 — Model Provider + Agent Runtime + Streaming

Closes: GAP 1 (agent runtime), GAP 8 (models package), GAP 10 (WebSocket session scoping).

---

### Task 6: `internal/models/` Base Package

**Files:**
- Create: `internal/models/models.go`
- Create: `internal/models/models_test.go`

**Step 1: Write failing test**

```go
// internal/models/models_test.go
package models_test

import (
    "testing"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

func TestContentBlockTypes_Bead_l3d_X_1(t *testing.T) {
    var _ models.ContentBlock = models.TextBlock{Text: "hello"}
    var _ models.ContentBlock = models.ToolUseBlock{ID: "1", Name: "read_file"}
    var _ models.ContentBlock = models.ToolResultBlock{ToolUseID: "1", Content: "ok"}
}

func TestStreamEventTypes_Bead_l3d_X_1(t *testing.T) {
    var _ models.StreamEvent = models.TextDeltaEvent{Delta: "hi"}
    var _ models.StreamEvent = models.ToolUseStartEvent{ID: "1", Name: "read_file"}
    var _ models.StreamEvent = models.MessageDeltaEvent{StopReason: "end_turn"}
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/models/... -v
```
Expected: `cannot find package`

**Step 3: Create `internal/models/models.go`**

```go
package models

import (
    "context"
    "encoding/json"
)

// ContentBlock is a sealed interface for message content parts.
type ContentBlock interface{ contentBlock() }

type TextBlock struct{ Text string }
func (TextBlock) contentBlock() {}

type ToolUseBlock struct {
    ID    string
    Name  string
    Input json.RawMessage
}
func (ToolUseBlock) contentBlock() {}

type ToolResultBlock struct {
    ToolUseID string
    Content   string
    IsError   bool
}
func (ToolResultBlock) contentBlock() {}

// Message is a single entry in a conversation.
type Message struct {
    Role    string // "user" or "assistant"
    Content []ContentBlock
}

// Tool describes a function the model may invoke.
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage
}

// StreamEvent is a sealed interface for streaming response chunks.
type StreamEvent interface{ streamEvent() }

type TextDeltaEvent struct{ Delta string }
func (TextDeltaEvent) streamEvent() {}

type ToolUseStartEvent struct{ ID, Name string }
func (ToolUseStartEvent) streamEvent() {}

type ToolInputDeltaEvent struct{ ID, Delta string }
func (ToolInputDeltaEvent) streamEvent() {}

type MessageDeltaEvent struct {
    OutputTokens int
    StopReason   string
}
func (MessageDeltaEvent) streamEvent() {}

type MessageStartEvent struct{ InputTokens int }
func (MessageStartEvent) streamEvent() {}

// Params configures a single model call.
type Params struct {
    Model     string
    Messages  []Message
    Tools     []Tool
    System    string
    MaxTokens int
}

// LanguageModel is the provider-agnostic single-call interface.
type LanguageModel interface {
    // Stream returns a channel of events. The channel closes when the
    // response is complete or ctx is cancelled.
    Stream(ctx context.Context, p Params) (<-chan StreamEvent, error)
    ModelID() string
}

// Provider creates LanguageModel instances by model ID.
type Provider interface {
    Name() string
    Model(id string) LanguageModel
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/models/... -v
```

**Step 5: Run full gate**
```bash
make format-check lint test build
```

**Step 6: Commit**
```bash
git add internal/models/
git commit -m "feat(models): add provider-agnostic LanguageModel interface [l3d.X.1]"
```

---

### Task 7: Provider Registry

**Files:**
- Create: `internal/models/registry.go`
- Create: `internal/models/registry_test.go`

**Step 1: Write failing test**

```go
func TestRegistry_RegisterAndResolve_Bead_l3d_X_1(t *testing.T) {
    reg := NewRegistry()
    mock := &mockProvider{name: "test"}
    reg.Register("test", mock)

    model, err := reg.Resolve("test", "test-model-1")
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if model.ModelID() != "test-model-1" {
        t.Errorf("got model ID %q, want %q", model.ModelID(), "test-model-1")
    }
}

func TestRegistry_ResolveUnknown_Bead_l3d_X_1(t *testing.T) {
    reg := NewRegistry()
    _, err := reg.Resolve("nonexistent", "x")
    if err == nil {
        t.Fatal("expected error for unknown provider")
    }
}

// mockProvider for tests
type mockProvider struct{ name string }
func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Model(id string) LanguageModel { return &mockModel{id: id} }

type mockModel struct{ id string }
func (m *mockModel) ModelID() string { return m.id }
func (m *mockModel) Stream(_ context.Context, _ Params) (<-chan StreamEvent, error) {
    ch := make(chan StreamEvent)
    close(ch)
    return ch, nil
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/models/... -run TestRegistry -v
```

**Step 3: Create `internal/models/registry.go`**

```go
package models

import (
    "fmt"
    "sync"
)

// Registry maps provider names to Provider instances.
type Registry struct {
    mu        sync.RWMutex
    providers map[string]Provider
}

func NewRegistry() *Registry {
    return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(name string, p Provider) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.providers[name] = p
}

// Resolve returns a LanguageModel for the given provider name and model ID.
func (r *Registry) Resolve(providerName, modelID string) (LanguageModel, error) {
    r.mu.RLock()
    p, ok := r.providers[providerName]
    r.mu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("provider %q not registered", providerName)
    }
    return p.Model(modelID), nil
}
```

**Step 4: Run to confirm pass + full gate**
```bash
go test ./internal/models/... -v
make format-check lint test build
```

**Step 5: Commit**
```bash
git add internal/models/registry.go internal/models/registry_test.go
git commit -m "feat(models): add provider registry [l3d.X.1]"
```

---

### Task 8: Anthropic Provider

**Files:**
- Create: `internal/models/anthropic/provider.go`
- Create: `internal/models/anthropic/provider_test.go`

**Step 1: Add the Anthropic SDK dependency**
```bash
go get github.com/anthropics/anthropic-sdk-go
```

**Step 2: Write test with a mock HTTP server (no real API calls)**

```go
// internal/models/anthropic/provider_test.go
package anthropic_test

import (
    "context"
    "testing"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
    anth "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models/anthropic"
)

func TestAnthropicProvider_Bead_l3d_X_1(t *testing.T) {
    p := anth.NewProvider("test-key")
    if p.Name() != "anthropic" {
        t.Errorf("got name %q, want %q", p.Name(), "anthropic")
    }
    m := p.Model("claude-sonnet-4-6")
    if m.ModelID() != "claude-sonnet-4-6" {
        t.Errorf("got model ID %q, want %q", m.ModelID(), "claude-sonnet-4-6")
    }
}

func TestAnthropicModel_StreamCancelledContext_Bead_l3d_X_1(t *testing.T) {
    p := anth.NewProvider("test-key")
    m := p.Model("claude-sonnet-4-6")

    ctx, cancel := context.WithCancel(context.Background())
    cancel() // cancel immediately

    ch, err := m.Stream(ctx, models.Params{
        Messages: []models.Message{{Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: "hi"}}}},
        MaxTokens: 10,
    })
    // Should either error immediately or return a closed channel
    if err == nil && ch != nil {
        // drain; channel must close
        for range ch {}
    }
}
```

**Step 3: Create `internal/models/anthropic/provider.go`**

```go
package anthropic

import (
    "context"
    "encoding/json"

    anthropicsdk "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/option"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

type Provider struct{ client *anthropicsdk.Client }

func NewProvider(apiKey string) *Provider {
    c := anthropicsdk.NewClient(option.WithAPIKey(apiKey))
    return &Provider{client: &c}
}

func (p *Provider) Name() string { return "anthropic" }
func (p *Provider) Model(id string) models.LanguageModel {
    return &model{client: p.client, id: id}
}

type model struct {
    client *anthropicsdk.Client
    id     string
}

func (m *model) ModelID() string { return m.id }

func (m *model) Stream(ctx context.Context, p models.Params) (<-chan models.StreamEvent, error) {
    ch := make(chan models.StreamEvent, 32)

    // Build SDK params
    sdkMessages := make([]anthropicsdk.MessageParam, len(p.Messages))
    for i, msg := range p.Messages {
        var parts []anthropicsdk.ContentBlockParamUnion
        for _, block := range msg.Content {
            switch b := block.(type) {
            case models.TextBlock:
                parts = append(parts, anthropicsdk.NewTextBlock(b.Text))
            case models.ToolResultBlock:
                parts = append(parts, anthropicsdk.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
            }
        }
        if msg.Role == "user" {
            sdkMessages[i] = anthropicsdk.NewUserMessage(parts...)
        } else {
            sdkMessages[i] = anthropicsdk.NewAssistantMessage(parts...)
        }
    }

    var sdkTools []anthropicsdk.ToolParam
    for _, t := range p.Tools {
        sdkTools = append(sdkTools, anthropicsdk.ToolParam{
            Name:        anthropicsdk.String(t.Name),
            Description: anthropicsdk.String(t.Description),
            InputSchema: anthropicsdk.ToolInputSchemaParam{Properties: t.InputSchema},
        })
    }

    params := anthropicsdk.MessageNewParams{
        Model:     anthropicsdk.Model(m.id),
        Messages:  sdkMessages,
        MaxTokens: int64(p.MaxTokens),
    }
    if p.System != "" {
        params.System = []anthropicsdk.TextBlockParam{{Text: anthropicsdk.String(p.System)}}
    }
    if len(sdkTools) > 0 {
        params.Tools = sdkTools
    }

    go func() {
        defer close(ch)
        stream := m.client.Messages.NewStreaming(ctx, params)
        for stream.Next() {
            event := stream.Current()
            switch e := event.AsUnion().(type) {
            case anthropicsdk.ContentBlockDeltaEvent:
                switch d := e.Delta.AsUnion().(type) {
                case anthropicsdk.TextDelta:
                    ch <- models.TextDeltaEvent{Delta: d.Text}
                case anthropicsdk.InputJSONDelta:
                    ch <- models.ToolInputDeltaEvent{Delta: d.PartialJSON}
                }
            case anthropicsdk.ContentBlockStartEvent:
                if tb, ok := e.ContentBlock.AsUnion().(anthropicsdk.ToolUseBlock); ok {
                    ch <- models.ToolUseStartEvent{ID: tb.ID, Name: tb.Name}
                }
            case anthropicsdk.MessageDeltaEvent:
                raw, _ := json.Marshal(e.Delta.StopReason)
                ch <- models.MessageDeltaEvent{
                    OutputTokens: int(e.Usage.OutputTokens),
                    StopReason:   string(raw),
                }
            case anthropicsdk.MessageStartEvent:
                ch <- models.MessageStartEvent{InputTokens: int(e.Message.Usage.InputTokens)}
            }
        }
    }()

    return ch, nil
}
```

**Note:** Exact SDK method names depend on the installed SDK version. Run
`go doc github.com/anthropics/anthropic-sdk-go` to verify types before implementing.

**Step 4: Run tests**
```bash
go test ./internal/models/anthropic/... -v
```

**Step 5: Full gate**
```bash
make format-check lint test build
```

**Step 6: Commit**
```bash
git add internal/models/anthropic/ go.mod go.sum
git commit -m "feat(models/anthropic): add Anthropic streaming provider [l3d.X.1]"
```

---

### Task 9: OpenAI Provider

**Files:**
- Create: `internal/models/openai/provider.go`
- Create: `internal/models/openai/provider_test.go`

**Step 1: Add dependency**
```bash
go get github.com/openai/openai-go
```

**Step 2: Write test (same pattern as Anthropic)**

```go
package openai_test

import (
    "testing"
    oai "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models/openai"
)

func TestOpenAIProvider_Bead_l3d_X_1(t *testing.T) {
    p := oai.NewProvider("test-key")
    if p.Name() != "openai" {
        t.Errorf("got name %q, want %q", p.Name(), "openai")
    }
    m := p.Model("gpt-4o")
    if m.ModelID() != "gpt-4o" {
        t.Errorf("got model ID %q, want %q", m.ModelID(), "gpt-4o")
    }
}
```

**Step 3: Create `internal/models/openai/provider.go`**

Follow the same pattern as the Anthropic provider: `NewProvider(apiKey)` returns a
`*Provider` implementing `models.Provider`. The `Stream` method uses the OpenAI Go SDK's
streaming Chat Completions API, mapping `delta.content` to `TextDeltaEvent` and
`delta.tool_calls` to `ToolUseStartEvent` / `ToolInputDeltaEvent`.

Run `go doc github.com/openai/openai-go` to confirm exact streaming types before
implementing.

**Step 4: Run gate**
```bash
make format-check lint test build
```

**Step 5: Commit**
```bash
git add internal/models/openai/ go.mod go.sum
git commit -m "feat(models/openai): add OpenAI streaming provider [l3d.X.1]"
```

---

### Task 10: WebSocket Session Scoping

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: relevant controlplane test file (search for `handleEventStream` tests)

**Step 1: Find the existing event stream tests**
```bash
grep -rn "EventStream\|event.stream\|events/stream" internal/controlplane/ --include="*_test.go"
```

**Step 2: Write failing test asserting 400 when session_id missing**

In the relevant test file, add:

```go
func TestEventStream_RequiresSessionID_Bead_l3d_X_1(t *testing.T) {
    srv := newTestServer(t)
    resp := srv.httpGet(t, "/api/v1/events/stream") // no session_id param
    if resp.StatusCode != http.StatusBadRequest {
        t.Errorf("expected 400, got %d", resp.StatusCode)
    }
}

func TestEventStream_RejectsUnknownSession_Bead_l3d_X_1(t *testing.T) {
    srv := newTestServer(t)
    resp := srv.httpGet(t, "/api/v1/events/stream?session_id=nonexistent")
    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("expected 404, got %d", resp.StatusCode)
    }
}
```

**Step 3: Modify `handleEventStream` in `server.go`**

At the top of the handler, before the WebSocket upgrade:

```go
sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
if sessionID == "" {
    writeStatusError(w, http.StatusBadRequest, "session_id query parameter is required")
    return
}
if _, err := s.sessionMgr.Get(r.Context(), sessionID); err != nil {
    writeStatusError(w, http.StatusNotFound, "session not found")
    return
}
```

Pass `sessionID` into `streamNewEvents` and update that function to filter by session:

```go
func (s *Server) streamNewEvents(ctx context.Context, conn *websocket.Conn, sessionID string, cursor eventStreamCursor) (eventStreamCursor, error) {
    events, err := s.eventStore.QueryBySession(ctx, sessionID)
    // ... rest of existing logic, already filtered to session
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/controlplane/... -run TestEventStream -v
```

**Step 5: Full gate + commit**
```bash
make format-check lint test build
git add internal/controlplane/server.go internal/controlplane/
git commit -m "feat(controlplane): scope WebSocket event stream to session_id [l3d.X.1]"
```

---

### Task 11: In-Memory Stream Bus

**Files:**
- Create: `internal/controlplane/streambus.go`
- Create: `internal/controlplane/streambus_test.go`

**Step 1: Write failing test**

```go
// internal/controlplane/streambus_test.go
package controlplane

import (
    "testing"
    "time"
)

func TestStreamBus_PublishSubscribe_Bead_l3d_X_1(t *testing.T) {
    bus := newStreamBus()
    ch, unsub := bus.subscribe("sess-1")
    defer unsub()

    bus.publish("sess-1", streamMessage{Type: "text_delta", Data: "hello"})

    select {
    case msg := <-ch:
        if msg.Data != "hello" {
            t.Errorf("got %q, want %q", msg.Data, "hello")
        }
    case <-time.After(100 * time.Millisecond):
        t.Fatal("timed out waiting for message")
    }
}

func TestStreamBus_NoLeakAcrossSessions_Bead_l3d_X_1(t *testing.T) {
    bus := newStreamBus()
    ch, unsub := bus.subscribe("sess-1")
    defer unsub()

    bus.publish("sess-2", streamMessage{Type: "text_delta", Data: "other"})

    select {
    case msg := <-ch:
        t.Fatalf("received unexpected message for wrong session: %v", msg)
    case <-time.After(20 * time.Millisecond):
        // correct: nothing received
    }
}

func TestStreamBus_Unsubscribe_Bead_l3d_X_1(t *testing.T) {
    bus := newStreamBus()
    _, unsub := bus.subscribe("sess-1")
    unsub() // unsubscribe before publish
    // should not panic or block
    bus.publish("sess-1", streamMessage{Type: "text_delta", Data: "after-unsub"})
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/controlplane/... -run TestStreamBus -v
```

**Step 3: Create `internal/controlplane/streambus.go`**

```go
package controlplane

import "sync"

type streamMessage struct {
    Type string
    Data string
}

type streamBus struct {
    mu   sync.RWMutex
    subs map[string][]chan streamMessage
}

func newStreamBus() *streamBus {
    return &streamBus{subs: make(map[string][]chan streamMessage)}
}

func (b *streamBus) subscribe(sessionID string) (<-chan streamMessage, func()) {
    ch := make(chan streamMessage, 64)
    b.mu.Lock()
    b.subs[sessionID] = append(b.subs[sessionID], ch)
    b.mu.Unlock()

    unsub := func() {
        b.mu.Lock()
        defer b.mu.Unlock()
        chans := b.subs[sessionID]
        for i, c := range chans {
            if c == ch {
                b.subs[sessionID] = append(chans[:i], chans[i+1:]...)
                close(ch)
                break
            }
        }
        if len(b.subs[sessionID]) == 0 {
            delete(b.subs, sessionID)
        }
    }
    return ch, unsub
}

func (b *streamBus) publish(sessionID string, msg streamMessage) {
    b.mu.RLock()
    chans := b.subs[sessionID]
    b.mu.RUnlock()
    for _, ch := range chans {
        select {
        case ch <- msg:
        default: // drop if subscriber is slow
        }
    }
}
```

**Step 4: Add `bus *streamBus` field to `Server` struct and initialise in `NewServer`**

```go
// in Server struct:
bus *streamBus

// in NewServer, after httpServer initialisation:
server.bus = newStreamBus()
```

**Step 5: Update `handleEventStream` to also drain the bus while waiting**

After the WebSocket upgrade, alongside the existing poll loop, also read from
`bus.subscribe(sessionID)` and forward messages immediately — providing low-latency
streaming while the event store remains the durable fallback.

```go
busC, busUnsub := s.bus.subscribe(sessionID)
defer busUnsub()

for {
    select {
    case msg, ok := <-busC:
        if !ok {
            return
        }
        payload, _ := json.Marshal(msg)
        if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
            return
        }
    case <-time.After(eventStreamPollInterval):
        // fall through to event store poll for reconnect durability
        newCursor, err := s.streamNewEvents(ctx, conn, sessionID, cursor)
        if err != nil {
            return
        }
        cursor = newCursor
    case <-ctx.Done():
        return
    }
}
```

**Step 6: Run tests + full gate**
```bash
go test ./internal/controlplane/... -run TestStreamBus -v
make format-check lint test build
```

**Step 7: Commit**
```bash
git add internal/controlplane/streambus.go internal/controlplane/streambus_test.go internal/controlplane/server.go
git commit -m "feat(controlplane): add session-scoped stream bus for agent token streaming [l3d.X.1]"
```

---

### Task 12: Agent Runtime — Config and Types

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`

**Step 1: Write failing type-assertion test**

```go
// internal/agent/agent_test.go
package agent_test

import (
    "testing"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
)

func TestAgentConfig_Defaults_Bead_l3d_X_1(t *testing.T) {
    cfg := agent.DefaultConfig("tier1")
    if cfg.MaxDepth <= 0 {
        t.Errorf("MaxDepth must be positive, got %d", cfg.MaxDepth)
    }
    if cfg.MaxToolCalls <= 0 {
        t.Errorf("MaxToolCalls must be positive, got %d", cfg.MaxToolCalls)
    }
}

func TestAgentConfig_Tier0_IsMoreRestrictive_Bead_l3d_X_1(t *testing.T) {
    tier0 := agent.DefaultConfig("tier0")
    tier2 := agent.DefaultConfig("tier2")
    if tier0.MaxDepth >= tier2.MaxDepth {
        t.Errorf("tier0 MaxDepth %d should be less than tier2 %d", tier0.MaxDepth, tier2.MaxDepth)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/agent/... -v
```

**Step 3: Create `internal/agent/agent.go`**

```go
package agent

import (
    "context"
    "time"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

// Config controls agent loop behaviour for a single turn.
type Config struct {
    MaxDepth      int
    MaxToolCalls  int
    MaxRetries    int
    TurnTimeout   time.Duration
    MaxOutputBytes int
}

// DefaultConfig returns per-tier agent limits.
func DefaultConfig(trustTier string) Config {
    switch trustTier {
    case "tier0":
        return Config{MaxDepth: 1, MaxToolCalls: 3, MaxRetries: 1, TurnTimeout: 30 * time.Second, MaxOutputBytes: 64 * 1024}
    case "tier1":
        return Config{MaxDepth: 3, MaxToolCalls: 10, MaxRetries: 2, TurnTimeout: 120 * time.Second, MaxOutputBytes: 256 * 1024}
    case "tier2":
        return Config{MaxDepth: 5, MaxToolCalls: 20, MaxRetries: 3, TurnTimeout: 300 * time.Second, MaxOutputBytes: 512 * 1024}
    default: // tier3
        return Config{MaxDepth: 10, MaxToolCalls: 50, MaxRetries: 3, TurnTimeout: 600 * time.Second, MaxOutputBytes: 1024 * 1024}
    }
}

// AgentTurn carries the input for a single agent turn.
type AgentTurn struct {
    ID        string
    SessionID string
    Message   string
    TrustTier string
    Files     []string
}

// AgentEventType identifies the kind of agent event.
type AgentEventType string

const (
    EventTextDelta         AgentEventType = "text_delta"
    EventToolUseStart      AgentEventType = "tool_use_start"
    EventToolInputDelta    AgentEventType = "tool_input_delta"
    EventTurnCompleted     AgentEventType = "turn_completed"
    EventTurnFailed        AgentEventType = "turn_failed"
    EventAwaitingApproval  AgentEventType = "awaiting_approval"
)

// AgentEvent is emitted by the runner to the stream bus and WebSocket.
type AgentEvent struct {
    Type      AgentEventType
    Data      string
    StopReason string
    ToolID    string
    ToolName  string
}

// ApprovalGate is implemented by the controlplane approval manager.
// The agent package does not import controlplane.
type ApprovalGate interface {
    RequestApproval(ctx context.Context, call tools.ToolCall) (ApprovalResult, error)
}

// ApprovalResult carries the approval decision.
type ApprovalResult struct {
    Approved bool
    Reason   string
}

// EventEmitter publishes agent events to subscribers (stream bus).
type EventEmitter interface {
    Emit(sessionID string, event AgentEvent)
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/agent/... -v
make format-check lint test build
```

**Step 5: Commit**
```bash
git add internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat(agent): add Config, AgentTurn, AgentEvent types [l3d.X.1]"
```

---

### Task 13: Agent Runtime — Runner and ReAct Loop

**Files:**
- Create: `internal/agent/runner.go`
- Create: `internal/agent/runner_test.go`

**Step 1: Write failing test with a mock model**

```go
// internal/agent/runner_test.go
package agent_test

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

// mockModel streams a fixed sequence of events then closes.
type mockModel struct {
    events []models.StreamEvent
    id     string
}

func (m *mockModel) ModelID() string { return m.id }
func (m *mockModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
    ch := make(chan models.StreamEvent, len(m.events)+1)
    for _, e := range m.events {
        ch <- e
    }
    close(ch)
    return ch, nil
}

type captureEmitter struct{ events []agent.AgentEvent }
func (c *captureEmitter) Emit(_ string, e agent.AgentEvent) { c.events = append(c.events, e) }

type noopGate struct{}
func (noopGate) RequestApproval(_ context.Context, _ tools.ToolCall) (agent.ApprovalResult, error) {
    return agent.ApprovalResult{Approved: true}, nil
}

func TestRunner_TextOnlyTurn_Bead_l3d_X_1(t *testing.T) {
    m := &mockModel{
        id: "mock",
        events: []models.StreamEvent{
            models.TextDeltaEvent{Delta: "Hello"},
            models.TextDeltaEvent{Delta: " world"},
            models.MessageDeltaEvent{StopReason: "end_turn", OutputTokens: 10},
        },
    }
    emitter := &captureEmitter{}
    reg := tools.NewRegistry()
    exec := tools.NewExecutor(reg, nil)

    runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, agent.DefaultConfig("tier1"))
    ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{
        ID: "t1", SessionID: "s1", Message: "say hi", TrustTier: "tier1",
    })
    if err != nil {
        t.Fatalf("RunTurn error: %v", err)
    }

    var completed bool
    timeout := time.After(2 * time.Second)
    for !completed {
        select {
        case evt, ok := <-ch:
            if !ok || evt.Type == agent.EventTurnCompleted {
                completed = true
            }
        case <-timeout:
            t.Fatal("timed out waiting for turn completion")
        }
    }

    // Verify text deltas were emitted
    textCount := 0
    for _, e := range emitter.events {
        if e.Type == agent.EventTextDelta {
            textCount++
        }
    }
    if textCount != 2 {
        t.Errorf("expected 2 text delta events, got %d", textCount)
    }
}

func TestRunner_MaxDepthEnforced_Bead_l3d_X_1(t *testing.T) {
    // Model always returns a tool use, which would loop forever without depth limit
    toolInput, _ := json.Marshal(map[string]string{"name": "test"})
    m := &mockModel{
        id: "mock",
        events: []models.StreamEvent{
            models.ToolUseStartEvent{ID: "tu1", Name: "echo"},
            models.ToolInputDeltaEvent{ID: "tu1", Delta: string(toolInput)},
            models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 5},
        },
    }
    emitter := &captureEmitter{}
    reg := tools.NewRegistry()
    reg.Register(tools.ToolDefinition{
        Name:   "echo",
        Limits: tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
    })
    exec := tools.NewExecutor(reg, map[string]tools.ToolHandler{
        "echo": func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
            return input, nil
        },
    })

    cfg := agent.DefaultConfig("tier0") // MaxDepth: 1
    runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)
    ch, _ := runner.RunTurn(context.Background(), agent.AgentTurn{
        ID: "t2", SessionID: "s1", Message: "loop", TrustTier: "tier0",
    })

    var stopReason string
    timeout := time.After(2 * time.Second)
    for {
        select {
        case evt, ok := <-ch:
            if !ok {
                goto done
            }
            if evt.Type == agent.EventTurnCompleted {
                stopReason = evt.StopReason
                goto done
            }
        case <-timeout:
            t.Fatal("timed out")
        }
    }
done:
    if stopReason != "max_depth" {
        t.Errorf("expected stop_reason max_depth, got %q", stopReason)
    }
}
```

**Step 2: Run to confirm failure**
```bash
go test ./internal/agent/... -run TestRunner -v
```

**Step 3: Create `internal/agent/runner.go`**

```go
package agent

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

// Runner executes the ReAct loop for a single turn.
type Runner struct {
    model    models.LanguageModel
    registry *tools.Registry
    executor *tools.Executor
    approval ApprovalGate
    emitter  EventEmitter
    cfg      Config
}

func NewRunner(
    model models.LanguageModel,
    registry *tools.Registry,
    executor *tools.Executor,
    approval ApprovalGate,
    emitter EventEmitter,
    cfg Config,
) *Runner {
    return &Runner{
        model:    model,
        registry: registry,
        executor: executor,
        approval: approval,
        emitter:  emitter,
        cfg:      cfg,
    }
}

// RunTurn starts the ReAct loop in a goroutine and returns a channel of events.
// The channel closes when the turn reaches a terminal state.
func (r *Runner) RunTurn(ctx context.Context, turn AgentTurn) (<-chan AgentEvent, error) {
    ch := make(chan AgentEvent, 64)
    go func() {
        defer close(ch)
        r.loop(ctx, turn, ch)
    }()
    return ch, nil
}

func (r *Runner) loop(ctx context.Context, turn AgentTurn, ch chan<- AgentEvent) {
    messages := []models.Message{
        {Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: turn.Message}}},
    }

    for depth := 0; depth < r.cfg.MaxDepth; depth++ {
        params := models.Params{
            Model:     r.model.ModelID(),
            Messages:  messages,
            MaxTokens: 4096,
        }

        // Register available tools in params
        all := r.registry.List()
        for _, def := range all {
            schema, _ := json.Marshal(def.InputSchema)
            params.Tools = append(params.Tools, models.Tool{
                Name:        def.Name,
                Description: def.Description,
                InputSchema: schema,
            })
        }

        stream, err := r.model.Stream(ctx, params)
        if err != nil {
            ch <- AgentEvent{Type: EventTurnFailed, Data: fmt.Sprintf("stream error: %v", err)}
            return
        }

        // Accumulate response
        var toolUses []accumulatedToolUse
        var currentTool *accumulatedToolUse
        stopReason := "end_turn"

        for event := range stream {
            switch e := event.(type) {
            case models.TextDeltaEvent:
                ch <- AgentEvent{Type: EventTextDelta, Data: e.Delta}
                r.emitter.Emit(turn.SessionID, AgentEvent{Type: EventTextDelta, Data: e.Delta})
            case models.ToolUseStartEvent:
                currentTool = &accumulatedToolUse{id: e.ID, name: e.Name}
                toolUses = append(toolUses, *currentTool)
                ch <- AgentEvent{Type: EventToolUseStart, ToolID: e.ID, ToolName: e.Name}
                r.emitter.Emit(turn.SessionID, AgentEvent{Type: EventToolUseStart, ToolID: e.ID, ToolName: e.Name})
            case models.ToolInputDeltaEvent:
                if currentTool != nil && currentTool.id == e.ID {
                    currentTool = &accumulatedToolUse{id: currentTool.id, name: currentTool.name, inputJSON: currentTool.inputJSON + e.Delta}
                    toolUses[len(toolUses)-1] = *currentTool
                }
                ch <- AgentEvent{Type: EventToolInputDelta, ToolID: e.ID, Data: e.Delta}
            case models.MessageDeltaEvent:
                stopReason = e.StopReason
            }
        }

        if len(toolUses) == 0 || stopReason == "end_turn" {
            ch <- AgentEvent{Type: EventTurnCompleted, StopReason: stopReason}
            r.emitter.Emit(turn.SessionID, AgentEvent{Type: EventTurnCompleted, StopReason: stopReason})
            return
        }

        // Execute tool uses and build next messages
        var assistantContent []models.ContentBlock
        var toolResultContent []models.ContentBlock

        for _, tu := range toolUses {
            var input map[string]interface{}
            _ = json.Unmarshal([]byte(tu.inputJSON), &input)

            call := tools.ToolCall{
                ID:        tu.id,
                ToolName:  tu.name,
                Input:     input,
                SessionID: turn.SessionID,
            }

            // Check approval
            ar, approvalErr := r.approval.RequestApproval(ctx, call)
            if approvalErr != nil || !ar.Approved {
                reason := "denied"
                if approvalErr != nil {
                    reason = approvalErr.Error()
                } else {
                    reason = ar.Reason
                }
                toolResultContent = append(toolResultContent, models.ToolResultBlock{
                    ToolUseID: tu.id,
                    Content:   fmt.Sprintf("approval denied: %s", reason),
                    IsError:   true,
                })
                continue
            }

            result, execErr := r.executor.Execute(ctx, call)
            var content string
            isError := false
            if execErr != nil {
                content = fmt.Sprintf("error: %v", execErr)
                isError = true
            } else if result != nil {
                out, _ := json.Marshal(result.Output)
                content = string(out)
            }

            assistantContent = append(assistantContent, models.ToolUseBlock{
                ID: tu.id, Name: tu.name,
                Input: json.RawMessage(tu.inputJSON),
            })
            toolResultContent = append(toolResultContent, models.ToolResultBlock{
                ToolUseID: tu.id,
                Content:   content,
                IsError:   isError,
            })
        }

        messages = append(messages,
            models.Message{Role: "assistant", Content: assistantContent},
            models.Message{Role: "user", Content: toolResultContent},
        )
    }

    // Depth limit reached
    ch <- AgentEvent{Type: EventTurnCompleted, StopReason: "max_depth"}
    r.emitter.Emit(turn.SessionID, AgentEvent{Type: EventTurnCompleted, StopReason: "max_depth"})
}

type accumulatedToolUse struct {
    id        string
    name      string
    inputJSON string
}
```

Add `List() []ToolDefinition` to `internal/tools/registry.go` if it doesn't exist:
```go
func (r *Registry) List() []ToolDefinition {
    r.mu.RLock()
    defer r.mu.RUnlock()
    out := make([]ToolDefinition, 0, len(r.tools))
    for _, v := range r.tools {
        out = append(out, v)
    }
    return out
}
```

**Step 4: Run to confirm pass**
```bash
go test ./internal/agent/... -v
```

**Step 5: Full gate**
```bash
make format-check lint test build
```

**Step 6: Commit**
```bash
git add internal/agent/ internal/tools/registry.go
git commit -m "feat(agent): add ReAct runner with trust-tier depth control [l3d.X.1]"
```

---

### Task 14: Wire Agent into Turn Creation

**Files:**
- Modify: `internal/controlplane/server.go`
- Test: `internal/controlplane/turn_lifecycle_test.go` (or equivalent)

**Step 1: Add `runner *agent.Runner` field to `Server` and initialise in `NewServer`**

After `toolExecutor` is wired, add:

```go
// Resolve model via route engine (use a sensible default model ID)
var agentModel models.LanguageModel
if s.routeEngine != nil {
    // Register providers (from env)
    modReg := models.NewRegistry()
    if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
        modReg.Register("anthropic", anthropicprovider.NewProvider(key))
    }
    if key := os.Getenv("OPENAI_API_KEY"); key != "" {
        modReg.Register("openai", openaiprovider.NewProvider(key))
    }
    // Default to anthropic claude-sonnet-4-6
    if m, err := modReg.Resolve("anthropic", "claude-sonnet-4-6"); err == nil {
        agentModel = m
    }
}
if agentModel != nil {
    cfg := agent.DefaultConfig("tier1") // server-level default; per-session override below
    s.runner = agent.NewRunner(agentModel, s.toolRegistry, s.toolExecutor, s, busEmitter{bus: s.bus}, cfg)
}
```

Define `busEmitter` (small adapter in `server.go`):
```go
type busEmitter struct{ bus *streamBus }
func (b busEmitter) Emit(sessionID string, e agent.AgentEvent) {
    b.bus.publish(sessionID, streamMessage{Type: string(e.Type), Data: e.Data})
}
```

**Step 2: Implement `ApprovalGate` on `Server`**

```go
func (s *Server) RequestApproval(ctx context.Context, call tools.ToolCall) (agent.ApprovalResult, error) {
    toolDef, ok := s.toolRegistry.Get(call.ToolName)
    if !ok || !toolDef.ApprovalRequired {
        return agent.ApprovalResult{Approved: true}, nil
    }
    // Propose approval and pause; this is async — for now return a pending signal
    // Full implementation: block goroutine on approval channel
    approval, err := s.approvalMgr.Propose(call)
    if err != nil {
        return agent.ApprovalResult{}, err
    }
    return agent.ApprovalResult{Approved: false, Reason: "approval_id:" + approval.ID}, nil
}
```

**Note:** Full approval-gate blocking (pausing the agent goroutine until the operator
decides) requires a per-approval channel. Implement as a `map[string]chan bool` in
`approvalMgr` and block the `RequestApproval` call until the channel resolves. This
is its own small design; stub `Approved: true` for non-approval-required tools first.

**Step 3: Invoke the runner when a turn is created**

In `handleCreateSessionTurn` (or wherever turns are created), after the turn is persisted:

```go
if s.runner != nil {
    // Derive per-session config from trust tier
    sess, _ := s.sessionMgr.Get(r.Context(), req.SessionID)
    cfg := agent.DefaultConfig(sess.TrustTier)
    // Rebuild runner with session-specific config
    sessionRunner := s.runner.WithConfig(cfg)
    go func() {
        ch, err := sessionRunner.RunTurn(r.Context(), agent.AgentTurn{
            ID:        turn.ID,
            SessionID: req.SessionID,
            Message:   req.Message,
            TrustTier: sess.TrustTier,
            Files:     req.Files,
        })
        if err != nil {
            log.Printf("agent RunTurn error: %v", err)
            return
        }
        for range ch {} // drain; events emitted to bus by runner
    }()
}
```

Add `WithConfig(cfg Config) *Runner` to `internal/agent/runner.go`:
```go
func (r *Runner) WithConfig(cfg Config) *Runner {
    cp := *r
    cp.cfg = cfg
    return &cp
}
```

**Step 4: Write integration smoke test**

In an appropriate test file:
```go
func TestTurnCreation_TriggersAgentRunner_Bead_l3d_X_1(t *testing.T) {
    // Create a server with a mock model that immediately completes
    // Create a session, then a turn
    // Assert that EventTurnCompleted is eventually in the event store
}
```

**Step 5: Full gate**
```bash
make format-check lint test build
```

**Step 6: Commit**
```bash
git add internal/controlplane/server.go internal/agent/runner.go
git commit -m "feat(controlplane): wire agent runner into turn lifecycle [l3d.X.1]"
```

---

## Phase 3 — Plumbing + UX

Closes: GAP 5 (`internal/trust/`), GAP 6 (SymbolicExecutor), GAP 7 (TUI diff renderer), GAP 9 (reliability limits), GAP 11 (session export).

---

### Task 15: `internal/trust/` Package

**Files:**
- Create: `internal/trust/policy.go`
- Create: `internal/trust/policy_test.go`
- Modify: `internal/sandbox/trust_handler.go`

**Step 1: Write failing test**

```go
// internal/trust/policy_test.go
package trust_test

import (
    "testing"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/trust"
    "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestDefaultPolicy_Tier0_RestrictsWrite_Bead_l3d_X_1(t *testing.T) {
    p := trust.DefaultPolicy()
    if p.RequiresApproval("tier0", tools.SideEffectWrite) != true {
        t.Error("tier0 write should require approval")
    }
}

func TestDefaultPolicy_PluginsAllowed_Bead_l3d_X_1(t *testing.T) {
    p := trust.DefaultPolicy()
    if p.PluginsAllowed("tier3") {
        t.Error("tier3 should not allow plugins")
    }
    if !p.PluginsAllowed("tier1") {
        t.Error("tier1 should allow plugins")
    }
}
```

**Step 2: Create `internal/trust/policy.go`**

```go
package trust

import "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"

type Policy interface {
    MaxLoopDepth(tier string) int
    MaxToolCalls(tier string) int
    AllowedSideEffects(tier string) []tools.SideEffect
    RequiresApproval(tier string, effect tools.SideEffect) bool
    PluginsAllowed(tier string) bool
}

type defaultPolicy struct{}

func DefaultPolicy() Policy { return defaultPolicy{} }

func (defaultPolicy) MaxLoopDepth(tier string) int {
    switch tier {
    case "tier0": return 1
    case "tier1": return 3
    case "tier2": return 5
    default:      return 10
    }
}

func (defaultPolicy) MaxToolCalls(tier string) int {
    switch tier {
    case "tier0": return 3
    case "tier1": return 10
    case "tier2": return 20
    default:      return 50
    }
}

func (defaultPolicy) AllowedSideEffects(tier string) []tools.SideEffect {
    switch tier {
    case "tier0": return []tools.SideEffect{tools.SideEffectNone, tools.SideEffectRead}
    default:      return []tools.SideEffect{tools.SideEffectNone, tools.SideEffectRead, tools.SideEffectWrite, tools.SideEffectExecute}
    }
}

func (defaultPolicy) RequiresApproval(tier string, effect tools.SideEffect) bool {
    if tier == "tier0" { return effect != tools.SideEffectNone && effect != tools.SideEffectRead }
    if tier == "tier2" || tier == "tier3" { return effect == tools.SideEffectWrite || effect == tools.SideEffectExecute || effect == tools.SideEffectNetwork }
    return effect == tools.SideEffectExecute || effect == tools.SideEffectNetwork
}

func (defaultPolicy) PluginsAllowed(tier string) bool {
    return tier != "tier3"
}
```

**Step 3: Update `sandbox/trust_handler.go` to delegate to `trust.DefaultPolicy()`**

Replace any hardcoded tier checks with calls to `trust.DefaultPolicy()`.

**Step 4: Update `agent.DefaultConfig` to use `trust.DefaultPolicy()`**

In `internal/agent/agent.go`:
```go
import "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/trust"

func DefaultConfig(trustTier string) Config {
    p := trust.DefaultPolicy()
    return Config{
        MaxDepth:     p.MaxLoopDepth(trustTier),
        MaxToolCalls: p.MaxToolCalls(trustTier),
        // ...
    }
}
```

**Step 5: Full gate + commit**
```bash
make format-check lint test build
git add internal/trust/ internal/sandbox/trust_handler.go internal/agent/agent.go
git commit -m "feat(trust): extract TrustPolicy into internal/trust, wire into agent and sandbox [l3d.X.1]"
```

---

### Task 16: Register SymbolicExecutor as a Tool

**Files:**
- Modify: `internal/controlplane/server.go`
- Test: `internal/controlplane/tools_endpoint_test.go` (or equivalent)

**Step 1: Write failing test asserting `symbolic_shell` appears in tool list**

```go
func TestTools_SymbolicShellRegistered_Bead_l3d_X_1(t *testing.T) {
    srv := newTestServer(t)
    resp := srv.httpGet(t, "/api/v1/tools")
    // parse JSON body into []tools.ToolDefinition
    // assert any entry has Name == "symbolic_shell"
    assertToolRegistered(t, resp, "symbolic_shell")
}
```

**Step 2: Add registration in `server.go`**

After the existing `executor.RegisterHandler("restricted_shell", ...)` call:

```go
symExec := sandbox.NewSymbolicCommandExecutor(sandbox.DefaultSymbolicCommands())
executor.RegisterHandler("symbolic_shell", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
    cmd, _ := input["command"].(string)
    result, err := symExec.Run(ctx, cmd, nil, sandbox.SubprocessOptions{WorkingDir: repoRoot})
    if err != nil {
        return nil, err
    }
    return map[string]interface{}{
        "stdout":    result.Stdout,
        "stderr":    result.Stderr,
        "exit_code": result.ExitCode,
    }, nil
})
registry.Register(tools.ToolDefinition{
    Name:            "symbolic_shell",
    Description:     "Runs a named CI command from the symbolic allowlist (go_test_all, go_vet_all, etc.)",
    Source:          tools.ToolSourceBuiltin,
    SideEffect:      tools.SideEffectExecute,
    ApprovalRequired: true,
    Limits:          tools.ExecutionLimits{TimeoutSeconds: 300, MaxOutputBytes: 512 * 1024},
    InputSchema:     map[string]interface{}{"command": map[string]interface{}{"type": "string", "description": "symbolic command name"}},
})
```

**Step 3: Full gate + commit**
```bash
make format-check lint test build
git add internal/controlplane/server.go
git commit -m "feat(controlplane): register symbolic_shell tool with allowlist enforcement [l3d.X.1]"
```

---

### Task 17: TUI Diff Renderer

**Files:**
- Modify: `pkg/tui/app.go`
- Modify: `pkg/tui/app_test.go`

**Step 1: Write failing test**

```go
func TestRenderFileMutation_Bead_l3d_X_1(t *testing.T) {
    event := audit.Event{
        EventType: audit.EventFileMutation,
        Payload: map[string]string{
            "path":        "internal/foo/bar.go",
            "type":        "update",
            "before_hash": "abc",
            "after_hash":  "def",
        },
    }
    app := NewApp(nil, nil, nil)
    out := app.renderFileMutation(event)
    if !strings.Contains(out, "bar.go") {
        t.Errorf("expected path in output, got %q", out)
    }
    if !strings.Contains(out, "update") {
        t.Errorf("expected mutation type in output, got %q", out)
    }
}
```

**Step 2: Add `renderFileMutation` to `pkg/tui/app.go`**

```go
func (a *App) renderFileMutation(event audit.Event) string {
    payload, ok := event.Payload.(map[string]string)
    if !ok {
        return fmt.Sprintf("[file_mutation] (unparseable payload)")
    }
    path := payload["path"]
    mutType := payload["type"]
    before := payload["before_hash"]
    after := payload["after_hash"]
    if before == "" && after == "" {
        return fmt.Sprintf("  ~ %s (%s)", path, mutType)
    }
    return fmt.Sprintf("  ~ %s (%s)\n    before: %s\n    after:  %s", path, mutType, before, after)
}
```

**Step 3: Use in `handleReplay`**

In the existing replay handler, after printing each event, check if it's a
`file_mutation` event and call `renderFileMutation` for a richer display:

```go
for _, event := range events {
    if event.EventType == audit.EventFileMutation {
        _ = a.writeln(a.renderFileMutation(event))
    } else {
        data, _ := json.MarshalIndent(event, "", "  ")
        _ = a.writeln(string(data))
    }
}
```

**Step 4: Full gate + commit**
```bash
make format-check lint test build
git add pkg/tui/app.go pkg/tui/app_test.go
git commit -m "feat(tui): render file_mutation events as structured diff summary [l3d.X.1]"
```

---

### Task 18: Session Export Endpoint

**Files:**
- Modify: `internal/controlplane/server.go`
- Test: `internal/controlplane/` (add to existing session tests)

**Step 1: Write failing test**

```go
func TestSessionExport_Bead_l3d_X_1(t *testing.T) {
    srv := newTestServer(t)
    sessID := srv.createSession(t, "/tmp/repo", "tier1")

    resp := srv.httpGet(t, "/api/v1/sessions/"+sessID+"/export")
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }

    var export map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&export)

    if export["schema_version"] != "1" {
        t.Errorf("expected schema_version 1, got %v", export["schema_version"])
    }
    if export["session"] == nil {
        t.Error("expected session field in export")
    }
    if export["events"] == nil {
        t.Error("expected events field in export")
    }
}

func TestSessionExport_NotFound_Bead_l3d_X_1(t *testing.T) {
    srv := newTestServer(t)
    resp := srv.httpGet(t, "/api/v1/sessions/nonexistent/export")
    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("expected 404, got %d", resp.StatusCode)
    }
}
```

**Step 2: Add `handleSessionExport` to `server.go`**

Add the route in the mux setup (inside the `handleSessionOrTurn` dispatch or as a
separate route — check the existing routing logic in `handleSessionByID` and add
an `export` path check):

```go
// in handleSessionByID, before the single-part check:
if len(parts) == 2 && parts[1] == "export" {
    s.handleSessionExport(w, r, parts[0])
    return
}
```

Implement the handler:

```go
func (s *Server) handleSessionExport(w http.ResponseWriter, r *http.Request, sessionID string) {
    if r.Method != http.MethodGet {
        writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
        return
    }

    sess, err := s.sessionMgr.Get(r.Context(), sessionID)
    if err != nil {
        writeStatusError(w, http.StatusNotFound, "session not found")
        return
    }

    events, err := s.eventStore.QueryBySession(r.Context(), sessionID)
    if err != nil {
        writeStatusError(w, http.StatusInternalServerError, err.Error())
        return
    }

    // Filter redaction events — replace with markers
    redactedCount := 0
    filtered := make([]audit.Event, 0, len(events))
    for _, e := range events {
        if e.EventType == audit.EventToolRedaction {
            redactedCount++
            filtered = append(filtered, audit.Event{
                ID:        e.ID,
                Timestamp: e.Timestamp,
                EventType: "[redacted]",
            })
        } else {
            filtered = append(filtered, e)
        }
    }

    export := map[string]interface{}{
        "schema_version": "1",
        "exported_at":    time.Now().UTC().Format(time.RFC3339),
        "session":        sess,
        "events":         filtered,
        "redaction_summary": map[string]interface{}{
            "redacted_count": redactedCount,
        },
    }

    if err := json.NewEncoder(w).Encode(export); err != nil {
        log.Printf("failed to encode export: %v", err)
    }
}
```

**Step 3: Full gate + commit**
```bash
make format-check lint test build
git add internal/controlplane/server.go
git commit -m "feat(controlplane): add GET /sessions/{id}/export with redaction safety [l3d.X.1]"
```

---

## Verification

After all tasks complete, run the full verification pipeline:

```bash
make verify-json BEAD=<phase-bead-id> GENERATE_EVIDENCE=1 CLOSURE_CHECK=1
```

For each bead, generate evidence and check closure eligibility before closing in `bd`:

```bash
make beads-check-closure BEAD=l3d.X
bd close SyndicateCode-l3d.X --reason "all gaps closed, evidence generated"
```

Final go/no-go check:

```bash
make go-no-go-report BEAD=<primary-bead>
```

All V1 criteria should now be met:

| # | Criterion | Closed by |
|---|-----------|-----------|
| 2 | File changes replayable | Task 3 + 4 |
| 3 | Policy enforcement below model layer | Task 12-14 |
| 6 | Trust tiers alter behaviour | Task 12 (depth control) + Task 15 |
| 8 | Session exports safe | Task 18 |
