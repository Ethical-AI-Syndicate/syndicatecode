# Diff Renderer and Integrated LSP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver a feature-rich, navigable diff experience in TUI plus integrated control-plane LSP APIs with auto language detection and robust failure handling.

**Architecture:** Control-plane owns LSP process lifecycle and normalized response contracts; TUI consumes APIs and renders interactive views. Rich diff rendering is driven by structured models (files/hunks/lines) instead of plain string formatting, with keyboard navigation and graceful fallbacks. Existing `replay`/`diff` behavior remains backward compatible while introducing opt-in rich mode commands.

**Tech Stack:** Go (`net/http`, JSON), existing control-plane and TUI packages, JSON-RPC over stdio for LSP servers, existing test stack (`go test`).

---

### Task 1: Define LSP API Types and Client Surface

**Files:**
- Create: `pkg/api/lsp.go`
- Modify: `pkg/tui/types.go`
- Modify: `pkg/tui/client.go`
- Test: `pkg/tui/client_test.go`

**Step 1: Write the failing test**

- Add tests in `pkg/tui/client_test.go` for:
  - diagnostics endpoint decode,
  - hover response decode,
  - definition response decode,
  - normalized severity/range fields.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui -run "LSP|Diagnostics|Hover|Definition"`
Expected: FAIL due missing client methods/types.

**Step 3: Write minimal implementation**

- Add typed request/response models in `pkg/api/lsp.go`.
- Extend `pkg/tui/client.go` with methods:
  - `GetDiagnostics(ctx, sessionID, path)`
  - `GetSymbols(ctx, sessionID, path)`
  - `GetHover(ctx, req)`
  - `GetDefinition(ctx, req)`
  - `GetReferences(ctx, req)`
  - `GetCompletions(ctx, req)`

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui -run "LSP|Diagnostics|Hover|Definition"`
Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/api/lsp.go pkg/tui/types.go pkg/tui/client.go pkg/tui/client_test.go
git commit -m "feat(api): add typed LSP client contracts"
```

### Task 2: Add Control-Plane LSP Broker Interfaces and Stubs

**Files:**
- Create: `internal/controlplane/lsp_broker.go`
- Create: `internal/controlplane/lsp_process.go`
- Create: `internal/controlplane/lsp_routes.go`
- Modify: `internal/controlplane/server.go`
- Test: `internal/controlplane/lsp_routes_test.go`

**Step 1: Write the failing test**

- Add endpoint tests in `internal/controlplane/lsp_routes_test.go` for:
  - diagnostics route validation,
  - hover route validation,
  - not-found/invalid-session behavior.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/controlplane -run "LSP|Diagnostics|Hover"`
Expected: FAIL due missing routes/handlers.

**Step 3: Write minimal implementation**

- Implement broker interface and in-memory session map stubs.
- Register new `/api/v1/lsp/*` routes in `server.go`.
- Return normalized placeholders from handlers using strict schema validation.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/controlplane -run "LSP|Diagnostics|Hover"`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/controlplane/lsp_broker.go internal/controlplane/lsp_process.go internal/controlplane/lsp_routes.go internal/controlplane/server.go internal/controlplane/lsp_routes_test.go
git commit -m "feat(controlplane): add LSP broker endpoints"
```

### Task 3: Implement Auto Language Detection and Server Selection

**Files:**
- Create: `internal/controlplane/language_detect.go`
- Modify: `internal/controlplane/lsp_broker.go`
- Test: `internal/controlplane/language_detect_test.go`

**Step 1: Write the failing test**

- Add table-driven tests for language detection by:
  - file extension,
  - repo markers (`go.mod`, `package.json`, `pyproject.toml`),
  - unknown fallback behavior.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/controlplane -run "LanguageDetect|LSPBroker"`
Expected: FAIL due no detection implementation.

**Step 3: Write minimal implementation**

- Implement deterministic detector returning language + preferred server executable.
- Wire broker lookup to detector before starting/reusing process.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/controlplane -run "LanguageDetect|LSPBroker"`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/controlplane/language_detect.go internal/controlplane/lsp_broker.go internal/controlplane/language_detect_test.go
git commit -m "feat(controlplane): auto-detect language for LSP routing"
```

### Task 4: Build Structured Diff Model in TUI

**Files:**
- Create: `pkg/tui/diff_model.go`
- Modify: `pkg/tui/app.go`
- Test: `pkg/tui/diff_model_test.go`
- Test: `pkg/tui/app_test.go`

**Step 1: Write the failing test**

- Add tests for model parsing and navigation:
  - next/prev file,
  - next/prev hunk,
  - boundary behavior,
  - malformed hunk fallback.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui -run "DiffModel|DiffNavigation|MalformedHunk"`
Expected: FAIL due missing model/navigation.

**Step 3: Write minimal implementation**

- Introduce typed structures for file/hunk/line.
- Add cursor/navigation methods.
- Keep existing text fallback path unchanged.

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui -run "DiffModel|DiffNavigation|MalformedHunk"`
Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/tui/diff_model.go pkg/tui/app.go pkg/tui/diff_model_test.go pkg/tui/app_test.go
git commit -m "feat(tui): add structured diff model and navigation core"
```

### Task 5: Add Rich Diff Rendering and Commands

**Files:**
- Create: `pkg/tui/diff_view.go`
- Modify: `pkg/tui/app.go`
- Modify: `pkg/tui/client.go`
- Test: `pkg/tui/app_test.go`

**Step 1: Write the failing test**

- Add command/output tests for:
  - `diff-rich <session_id> [event_type]`,
  - fold/expand behavior markers,
  - file/hunk header readability output.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui -run "DiffRich|Fold|HunkHeader"`
Expected: FAIL due missing command/renderer.

**Step 3: Write minimal implementation**

- Add rich diff renderer with file grouping and hunk formatting.
- Wire new command and command mapping.
- Preserve `diff` legacy behavior unchanged.

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui -run "DiffRich|Fold|HunkHeader"`
Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/tui/diff_view.go pkg/tui/app.go pkg/tui/client.go pkg/tui/app_test.go
git commit -m "feat(tui): add rich diff command and renderer"
```

### Task 6: Add LSP TUI Commands and Output Views

**Files:**
- Create: `pkg/tui/lsp_view.go`
- Modify: `pkg/tui/app.go`
- Modify: `pkg/tui/app_test.go`

**Step 1: Write the failing test**

- Add tests for commands:
  - `diag`, `sym`, `hover`, `def`.
- Validate command parsing, endpoint invocation, and rendering format.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/tui -run "Diag|Sym|Hover|Def"`
Expected: FAIL due missing command handlers.

**Step 3: Write minimal implementation**

- Add command parsing and bindings in app command switch.
- Add concise renderers for diagnostics/symbols/position outputs.

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/tui -run "Diag|Sym|Hover|Def"`
Expected: PASS.

**Step 5: Commit**

```bash
git add pkg/tui/lsp_view.go pkg/tui/app.go pkg/tui/app_test.go
git commit -m "feat(tui): add integrated LSP commands and views"
```

### Task 7: Reliability and Error Policy for LSP

**Files:**
- Modify: `internal/controlplane/lsp_broker.go`
- Modify: `internal/controlplane/lsp_process.go`
- Modify: `internal/controlplane/lsp_routes.go`
- Test: `internal/controlplane/lsp_broker_test.go`
- Test: `internal/controlplane/lsp_routes_test.go`

**Step 1: Write the failing test**

- Add tests for:
  - missing server binary -> typed error,
  - startup timeout/crash -> retry then unhealthy,
  - request timeout -> stale/partial result handling.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/controlplane -run "LSPBroker|LSPError|LSPTimeout"`
Expected: FAIL due missing policy handling.

**Step 3: Write minimal implementation**

- Implement retry-once startup policy and typed error envelopes.
- Add timeout handling with stale markers for cacheable methods.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/controlplane -run "LSPBroker|LSPError|LSPTimeout"`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/controlplane/lsp_broker.go internal/controlplane/lsp_process.go internal/controlplane/lsp_routes.go internal/controlplane/lsp_broker_test.go internal/controlplane/lsp_routes_test.go
git commit -m "fix(lsp): add reliability policy and typed error behavior"
```

### Task 8: Metrics, Audit Events, and Schema Contracts

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: `internal/controlplane/schema_validation.go`
- Modify: `internal/validation/registry.go`
- Test: `internal/controlplane/schema_validation_test.go`

**Step 1: Write the failing test**

- Add schema tests for all new `/api/v1/lsp/*` responses.
- Add metrics exposure assertions for LSP request counters/latency buckets.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/controlplane ./internal/validation -run "Schema|LSP|Metrics"`
Expected: FAIL due missing schema/metrics entries.

**Step 3: Write minimal implementation**

- Register response schemas for new endpoints.
- Add LSP metric fields and increment points.
- Add lightweight audit event emission for LSP method invocations.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/controlplane ./internal/validation -run "Schema|LSP|Metrics"`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/controlplane/server.go internal/controlplane/schema_validation.go internal/validation/registry.go internal/controlplane/schema_validation_test.go
git commit -m "feat(observability): add LSP metrics and contract validation"
```

### Task 9: Documentation and Final Verification

**Files:**
- Modify: `docs/reference/cli-commands.md`
- Modify: `docs/how-to/review-session-history.md`
- Create: `docs/how-to/use-lsp-commands.md`
- Create: `docs/plans/2026-03-12-diff-renderer-lsp-verification.md`

**Step 1: Write verification checklist doc**

- Map implemented capabilities to tests and commands.

**Step 2: Run complete verification**

Run: `go test ./...`
Expected: PASS.

**Step 3: Run focused command-path checks**

Run: `go test ./pkg/tui ./internal/controlplane -run "DiffRich|Diag|Sym|Hover|Def|LSP"`
Expected: PASS.

**Step 4: Update docs with exact commands and failure modes**

- Document command syntax, examples, and typed error outputs.

**Step 5: Commit**

```bash
git add docs/reference/cli-commands.md docs/how-to/review-session-history.md docs/how-to/use-lsp-commands.md docs/plans/2026-03-12-diff-renderer-lsp-verification.md
git commit -m "docs: add rich diff and integrated lsp usage and verification"
```

## Implementation Notes

- Keep changes additive and backward-compatible for existing replay/diff behavior.
- Avoid introducing language-specific code paths in TUI; keep language handling in control-plane broker.
- Use fake backends in tests to avoid external LSP binary dependency in CI.
- Keep error responses typed and actionable.
