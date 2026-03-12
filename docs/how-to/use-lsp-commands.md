# Use LSP Commands in TUI

This guide covers the integrated LSP commands exposed by the SyndicateCode TUI.

## Prerequisites

- A running control-plane server.
- An active session with a valid repository path.
- A language server executable available for your codebase (for example `gopls`, `typescript-language-server`, `pylsp`).

## Commands

### Diagnostics

```
diag <session_id> <path>
```

Example:

```
diag s-123 internal/controlplane/lsp_routes.go
```

### Symbols

```
sym <session_id> <path>
```

### Hover

```
hover <session_id> <path> <line> <col>
```

### Definition

```
def <session_id> <path> <line> <col>
```

## Error behavior

LSP routes return typed error envelopes for backend failures:

- `lsp_server_unavailable`: server binary is missing/not resolvable.
- `lsp_backend_unhealthy`: backend startup failed and was marked unhealthy.
- `lsp_request_timeout`: request deadline reached.

Each envelope includes `type`, `reason`, and `retryable` fields.

## Notes

- Diagnostics/symbols may return cached results after request timeouts.
- Successful LSP calls emit `lsp_request` events and are visible via replay filtering.
