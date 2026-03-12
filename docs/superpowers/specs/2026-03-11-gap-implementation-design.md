# Gap Implementation Design
**Date:** 2026-03-11
**Status:** Approved
**Scope:** Close all 11 gaps identified in the 2026-03-11 production code audit

---

## Context

A production audit identified 11 gaps between the architectural specification
(`docs/ai_cli_v_1_architecture_checklist.md`, `docs/ai_cli_v_1_implementation_plan.md`,
and related docs) and the current codebase. This design closes them in three
sequentially-gated phases.

### Identified Gaps

| # | Gap | Severity |
|---|-----|----------|
| 1 | `internal/agent/` package entirely absent | Critical |
| 2 | `tool_invocations`, `model_invocations`, `file_mutations` tables never written | High |
| 3 | `model_invocation`, `tool_invocation`, `tool_result`, `file_mutation` event types missing | High |
| 4 | `BudgetAllocator` implemented but never wired | Medium |
| 5 | `internal/trust/` package empty | Medium |
| 6 | `SymbolicExecutor` never integrated | Low |
| 7 | TUI has no diff renderer | Low |
| 8 | `internal/models/` package never created | Low |
| 9 | Per-turn reliability limits not enforced | Low |
| 10 | WebSocket stream not session-scoped | Low |
| 11 | No session export endpoint | Low |

### Implementation Markers (2026-03-12 Reconciliation)

| Gap | Marker | Evidence |
|-----|--------|----------|
| 10 | Implemented | `internal/controlplane/server.go` (`handleEventStream`, `streamNewEvents`) + `internal/controlplane/event_stream_test.go` |
| 11 | Implemented | `internal/controlplane/server.go` (`handleSessionExport`, `parseIncludeArtifactsParam`) + `internal/controlplane/replay_test.go` |
| 5, 6, 7, 9 | Pending marker update in this document | No additional reconciliation changes in this pass |

---

## Design Decisions

- **Provider model:** Provider-agnostic, following Vercel AI SDK `LanguageModel` interface
  patterns implemented natively in Go with concrete Anthropic and OpenAI HTTP clients.
- **Agent loop:** ReAct-style with configurable depth (`C`). Trust tier controls
  `MaxDepth`: tier0→1, tier1→3, tier2→5, tier3→10.
- **Streaming:** Token-by-token via an in-memory `streamBus` broadcast to session-scoped
  WebSocket subscribers. Token deltas are bus-only (not persisted); durable events
  written to the event store as before.

---

## Phase 1 — Persistence Completeness

**Closes:** GAPs 2, 3, 4

### New Event Type Constants

Add to `internal/audit/event_types.go`:

```go
EventModelInvoked = "model_invocation"
EventToolInvoked  = "tool_invocation"
EventToolResult   = "tool_result"
EventFileMutation = "file_mutation"
```

### New EventStore Write Methods

Add to `internal/audit/event_store.go`:

```go
RecordToolInvocation(ctx context.Context, r ToolInvocationRecord) error
RecordModelInvocation(ctx context.Context, r ModelInvocationRecord) error
RecordFileMutation(ctx context.Context, r FileMutationRecord) error
```

Record structs mirror existing DB column layouts exactly — no schema migrations required.

### Wiring Points

| Component | Change |
|-----------|--------|
| `internal/tools/executor.go` | Call `RecordToolInvocation` before execution, `RecordToolResult` after |
| `internal/tools/patch_handler.go` | Call `RecordFileMutation` per operation in a successful `apply_patch` |
| `internal/context/context.go` `TurnManager.AssembleContext` | Call `NewBudgetAllocator` + `AllocateFragments` before returning fragments |

### Testing

- Table-driven bead-tagged tests for each new `EventStore` write method
- `executor_test.go` extended: assert `ToolInvocationRecord` written on execute
- `context_test.go` extended: assert budget allocation applied to fragment set

---

## Phase 2 — Model Provider + Agent Runtime + Streaming

**Closes:** GAPs 1, 8, 10 (and enables full V1 go/no-go compliance)

### 2a. `internal/models/` — Provider-Agnostic Interface

Package defines interfaces and shared types only. No HTTP code in the base package.

#### Core Types

```go
// Content block sum type (sealed via unexported marker method)
type ContentBlock interface{ contentBlock() }
type TextBlock       struct{ Text string }
type ToolUseBlock    struct{ ID, Name string; Input json.RawMessage }
type ToolResultBlock struct{ ToolUseID, Content string; IsError bool }

// Stream event sum type
type StreamEvent interface{ streamEvent() }
type TextDeltaEvent    struct{ Delta string }
type ToolUseStartEvent struct{ ID, Name string }
type ToolInputDelta    struct{ ID, Delta string }
type MessageDelta      struct{ OutputTokens int; StopReason string }

// Single model call interface
type LanguageModel interface {
    Stream(ctx context.Context, p Params) (<-chan StreamEvent, error)
    ModelID() string
}

// Provider factory
type Provider interface {
    Name() string
    Model(id string) LanguageModel
}
```

#### Params Struct

Carries: `Messages []Message`, `Tools []ToolDefinition`, `System string`,
`MaxTokens int`, `Temperature float64`. No agent-loop logic.

#### Provider Registry

`internal/models/registry.go`:
- `Register(name string, p Provider)`
- `Resolve(routeDecision policy.RouteDecision) (LanguageModel, error)`

The existing `RouteEngine` output feeds directly into `Resolve`.

#### Concrete Implementations

| Package | Dependency | Maps to |
|---------|-----------|---------|
| `internal/models/anthropic/` | `github.com/anthropics/anthropic-sdk-go` | Messages API streaming |
| `internal/models/openai/` | `github.com/openai/openai-go` | Chat Completions streaming |

### 2b. `internal/agent/` — ReAct Loop

```go
type Config struct {
    MaxDepth      int
    MaxToolCalls  int
    StreamTimeout time.Duration
}

// Interfaces — no import of controlplane
type ApprovalGate interface {
    RequestApproval(ctx context.Context, call tools.ToolCall) (ApprovalResult, error)
}
type EventEmitter interface {
    Emit(sessionID string, event AgentEvent)
}

type Runner struct {
    model    models.LanguageModel
    registry *tools.Registry
    executor *tools.Executor
    approval ApprovalGate
    emitter  EventEmitter
    cfg      Config
}

// RunTurn runs the ReAct loop in a goroutine.
// Returns a channel that closes on terminal state.
func (r *Runner) RunTurn(ctx context.Context, turn AgentTurn) (<-chan AgentEvent, error)
```

#### Loop Per Iteration

1. Assemble messages from turn context + prior tool results
2. Call `model.Stream()` — pipe `TextDeltaEvent` directly to emitter
3. Accumulate `ToolUseBlock` responses until stream closes
4. For each tool: check `ApprovalGate` → if approval required, pause and emit
   `turn_awaiting_approval` → resume on approval decision
5. Execute approved tools via `executor`, append `ToolResultBlock` to messages
6. Increment depth; if `depth >= MaxDepth` → emit `turn_completed` with
   `stop_reason: max_depth`
7. On `end_turn` stop reason → emit `turn_completed`

#### Trust-Tier Depth Mapping

Wired in server from `internal/trust/` policy (not hardcoded in agent):

| Tier | MaxDepth | MaxToolCalls |
|------|----------|--------------|
| tier0 | 1 | 3 |
| tier1 | 3 | 10 |
| tier2 | 5 | 20 |
| tier3 | 10 | 50 |

### 2c. Streaming Infrastructure

#### WebSocket Session Scoping [Implemented]

Fix GAP 10 as a prerequisite:
- `GET /api/v1/events/stream?session_id=<id>` — `session_id` becomes required
- `streamNewEvents()` filters event store queries by session ID
- Missing `session_id` returns `400 Bad Request`

#### In-Memory Stream Bus [Implemented]

```go
type streamBus struct {
    mu   sync.RWMutex
    subs map[string][]chan AgentEvent  // keyed by session_id
}

func (b *streamBus) Subscribe(sessionID string) (<-chan AgentEvent, func())
func (b *streamBus) Publish(sessionID string, event AgentEvent)
```

- Agent goroutine writes to `streamBus.Publish`
- WebSocket handler calls `Subscribe`, forwards events to client
- On client reconnect: WebSocket falls back to event store replay from last
  received event ID (resumability without re-running the loop)
- Bus is in-memory only — events are ephemeral at the bus layer;
  durable record is always the event store

#### Audit Events During Streaming

| Moment | Event Written |
|--------|--------------|
| Loop start | `EventModelInvoked` (provider, model, input tokens) |
| Tool execute | `EventToolInvoked` / `EventToolResult` (via Phase 1 paths) |
| File mutation | `EventFileMutation` (via Phase 1 patch_handler path) |
| Token deltas | Bus-only — **not** persisted |

---

## Phase 3 — Plumbing + UX

**Closes:** GAPs 5, 6, 7, 9, 11

### 3a. `internal/trust/` — Extract Trust Resolution

Define a proper package rather than scattered tier checks:

```go
type TrustPolicy interface {
    MaxLoopDepth(tier string) int
    MaxToolCalls(tier string) int
    AllowedSideEffects(tier string) []tools.SideEffect
    RequiresApproval(tier string, effect tools.SideEffect) bool
    PluginsAllowed(tier string) bool
}
```

`DefaultTrustPolicy` implements current hardcoded rules.
`sandbox/trust_handler.go` delegates to `trust.DefaultTrustPolicy`.
Server and agent runner import `internal/trust` for all tier decisions.

### 3b. `SymbolicExecutor` Integration

- Register `NewSymbolicCommandExecutor(DefaultSymbolicCommands())` in `server.go`
  as a `"symbolic_shell"` tool handler
- Add `ToolDefinition`: `SideEffect: Execute`, `ApprovalRequired: true`,
  `TrustLevel: tier1+`
- Symbolic allowlist (`go_test_all`, `go_vet_all`, etc.) is the enforced
  constraint — stricter than `restricted_shell` for known CI operations

### 3c. TUI Diff Renderer

When `diff` or `replay` output contains `file_mutation` events:
- Render patch operations as a colour unified diff (red/green lines, `@@` hunks)
- Fallback to structured text (path, mutation_type, hash pair) when artifact
  content is unavailable
- Implemented as `renderFileMutation(event audit.Event) string` in `pkg/tui/app.go`
- No new dependencies

### 3d. Session Export Endpoint [Implemented]

```
GET /api/v1/sessions/{id}/export
```

Response schema:
```json
{
  "schema_version": "1",
  "exported_at": "RFC3339",
  "session": {},
  "events": [],
  "redaction_summary": { "redacted_count": 0, "reason": "" }
}
```

Safety controls (V1 go/no-go #8):
- Events pass through `contextRedactionPolicy` before inclusion
- `EventToolRedaction` events replaced with redaction markers
- Artifacts excluded by default; `?include_artifacts=true` requires operator role

Reconciled implementation evidence:
- `internal/controlplane/server.go`: `handleSessionExport` parses `include_artifacts`
  and gates artifact inclusion behind operator role enforcement.
- `internal/controlplane/replay_test.go`: explicit coverage for non-operator `403` and
  operator `200` artifact export behavior.

### 3e. Per-Turn Reliability Limits

Add to `agent.Config` (set from `internal/trust/` policy):
```go
MaxModelRetries  int           // retry on transient provider errors only
TurnTimeout      time.Duration // context deadline on full RunTurn goroutine
MaxOutputBytes   int           // per-turn aggregate output cap
```

`MaxDepth` and `MaxToolCalls` set in Phase 2; Phase 3 adds the remaining three.

---

## Package Dependency Graph (Post-Implementation)

```
cmd/server
  └── internal/controlplane
        ├── internal/agent
        │     ├── internal/models        (new)
        │     │     ├── models/anthropic (new)
        │     │     └── models/openai    (new)
        │     ├── internal/trust         (extracted)
        │     ├── internal/tools
        │     └── internal/audit
        ├── internal/trust               (extracted)
        ├── internal/session
        ├── internal/context
        │     └── internal/context/budget_allocator (wired)
        ├── internal/audit               (+ 3 new write methods)
        └── internal/sandbox
              └── internal/trust         (delegates)
```

No new circular dependencies. All new packages respect existing boundary rules.

---

## V1 Go/No-Go Criteria — Post-Implementation Status

| Criterion | Status After |
|-----------|-------------|
| 1. Context inspectable | ✅ Already met |
| 2. File changes replayable | ✅ Phase 1 (file_mutation events + DB writes) |
| 3. Policy enforcement below model layer | ✅ Phase 2 (agent loop respects approval gate) |
| 4. Secrets filtered before model egress | ✅ Already met |
| 5. Shell execution constrained | ✅ Already met |
| 6. Trust tiers alter behavior | ✅ Phase 2 (depth/tool limits per tier) |
| 7. Approvals bind to exact actions | ✅ Already met |
| 8. Session exports safe | ✅ Phase 3 (redacted export endpoint) |
