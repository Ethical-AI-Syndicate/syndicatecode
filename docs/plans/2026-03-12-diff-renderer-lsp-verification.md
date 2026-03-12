# Diff Renderer + LSP Verification (2026-03-12)

## Scope

- Rich diff model and `diff-rich` rendering path in TUI.
- LSP API contracts, TUI commands, control-plane routes.
- LSP reliability policy (typed errors, startup retry, unhealthy backend behavior).
- LSP metrics, audit events, and schema contracts.

## Commands Executed

```bash
go test ./internal/controlplane ./internal/validation ./internal/audit
go test ./pkg/tui
go test ./...
```

## Coverage Highlights

- `internal/controlplane/lsp_broker_test.go`
  - Missing binary -> `lsp_server_unavailable`
  - Startup retry(2) then unhealthy -> `lsp_backend_unhealthy`
  - Timeout fallback to cached diagnostics
- `internal/controlplane/lsp_routes_test.go`
  - Request validation behavior
  - Unknown session behavior
  - Typed LSP envelope response
  - `lsp_request` audit emission on success
- `internal/controlplane/lsp_metrics_test.go`
  - Request and typed error counters in runtime metrics snapshot
- `internal/controlplane/schema_validation_test.go`
  - LSP position schema validation (missing line/col)

## Manual Command Path Checks

- `diff-rich <session_id> [event_type]`
- `diag <session_id> <path>`
- `sym <session_id> <path>`
- `hover <session_id> <path> <line> <col>`
- `def <session_id> <path> <line> <col>`

## Expected Failure Modes

- Missing executable: 503 with `type=lsp_server_unavailable`
- Unhealthy backend: 503 with `type=lsp_backend_unhealthy`
- LSP timeout: typed timeout error (and cache fallback where available)
