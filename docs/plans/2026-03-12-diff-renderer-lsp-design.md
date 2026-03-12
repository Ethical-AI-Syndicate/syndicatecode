# Feature-Rich Diff Renderer and Integrated LSP Design

Date: 2026-03-12
Scope: Control-plane + TUI
Language support strategy: Auto-detect by repository files
Primary UX priority: Readability + navigation

## Objective

Introduce a complete, feature-rich diff rendering experience and integrated Language Server Protocol (LSP) capabilities while preserving the control-plane-as-authority architecture.

## Architecture

- Add an LSP broker subsystem inside control-plane as the canonical service for language intelligence.
- Keep TUI as a client-only rendering and interaction layer.
- Replace current plain replay diff formatting with a structured diff model that supports navigation and folding.
- Preserve compatibility with current `replay`/`diff` flows, while adding a richer mode and LSP commands.

## Components and API Surface

### Control-plane components

- `internal/controlplane/lsp_broker.go`
  - broker interface, workspace/language session cache, lifecycle policy
- `internal/controlplane/lsp_process.go`
  - JSON-RPC stdio transport and process supervision
- `internal/controlplane/lsp_routes.go`
  - `GET /api/v1/lsp/diagnostics?session_id&path`
  - `GET /api/v1/lsp/symbols?session_id&path`
  - `POST /api/v1/lsp/hover`
  - `POST /api/v1/lsp/definition`
  - `POST /api/v1/lsp/references`
  - `POST /api/v1/lsp/completions`
- `internal/controlplane/diff_routes.go`
  - `GET /api/v1/sessions/{session_id}/diff?mode=rich`

### TUI components

- `pkg/tui/diff_model.go`
  - file/hunk/line structures and cursor state
- `pkg/tui/diff_view.go`
  - rich rendering pipeline with fold/expand and hunk navigation
- `pkg/tui/lsp_view.go`
  - diagnostics/symbol display and position-based query UX
- `pkg/tui/app.go`
  - new commands:
    - `diff-rich <session_id> [event_type]`
    - `diag <session_id> <path>`
    - `sym <session_id> <path>`
    - `hover <session_id> <path> <line> <col>`
    - `def <session_id> <path> <line> <col>`

### Response normalization

- LSP results are normalized in control-plane (ranges, severities, symbol kinds, hover payload shape).
- TUI consumes stable normalized contracts.

## Data Flow and Interaction Model

### LSP request flow

1. TUI issues command with `session_id` and file/position context.
2. Control-plane resolves repository root from session.
3. Broker auto-detects language and selects/starts appropriate LSP server.
4. Broker executes request and returns normalized response.
5. TUI renders result in command output or contextual panel.

### Rich diff flow

1. TUI requests rich diff data for a session.
2. Control-plane returns structured diffs (files/hunks/lines + metadata).
3. TUI builds navigable state model and renders:
   - file list,
   - active hunk,
   - optional diagnostics badges.

### Navigation keymap model

- `]f` / `[f`: next/previous file
- `]h` / `[h`: next/previous hunk
- `za`: fold toggle
- `gd`: go-to-definition at cursor (if LSP context available)
- `K`: hover at cursor

### Language auto-detection

- Detect from extension and workspace signals (examples: `go.mod`, `package.json`, `pyproject.toml`).
- Route to installed server when available.

## Error Handling, Reliability, and Safety

### LSP failure behavior

- Missing server binary -> `lsp_server_unavailable` with install hint.
- Startup crash/timeout -> one retry, then `lsp_backend_unhealthy`.
- Request timeout -> best-effort partial/cached response when available, marked `stale=true`.

### Safety boundaries

- LSP server execution remains inside control-plane constraints (repo-root bounded paths, env filtering, timeout caps).
- New endpoints follow existing session authorization and role rules.
- No direct shell passthrough from TUI for LSP operations.

### Diff robustness

- Malformed hunk payloads degrade to deterministic textual summary.
- Large diff payloads use paging windows and truncation indicators.
- Ordering remains deterministic by file path and hunk location.

### Observability

- Add metrics for LSP request count/latency/errors and broker restart events.
- Add read-only LSP audit metadata events (no sensitive content persistence by default).

## Testing and Acceptance

### Unit tests

- LSP response normalization and parser tests.
- Broker lifecycle tests (start/reuse/expire/restart).
- Diff cursor and folding behavior tests.

### Integration tests

- Control-plane LSP endpoint tests with fake backend.
- TUI command wiring tests for `diff-rich`, `diag`, `sym`, `hover`, `def`.
- Combined test: rich diff + diagnostics overlay for changed files.

### Regression tests

- Existing `replay`/`diff` behavior preserved.
- Existing malformed hunk fallback semantics preserved.

### Acceptance criteria

- Rich diff supports multi-file navigation and hunk folding.
- LSP endpoints function for auto-detected installed servers.
- Missing server/tooling errors are actionable and non-fatal.
- Full suite passes: `go test ./...`.
