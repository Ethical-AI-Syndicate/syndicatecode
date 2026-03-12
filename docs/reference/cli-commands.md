# CLI Command Reference

Complete reference for SyndicateCode TUI commands.

## Getting Help

### help

Display available commands.

```
help
```

## Session Commands

### start

Create a new session.

```
start <repo_path> [trust_tier]
```

Arguments:
- `repo_path` - Path to Git repository (required)
- `trust_tier` - Trust tier: tier0, tier1, tier2, tier3 (default: tier1)

Example:
```
start /home/user/myproject tier2
```

### sessions

List all sessions.

```
sessions
```

Options:
- `--status active|completed|terminated` - Filter by status

Example:
```
sessions --status active
```

### ask

Send a message to the agent.

```
ask <message>
```

Arguments:
- `message` - Your request (can be multi-word)

Example:
```
ask Fix the null pointer exception in auth.go
```

### context

View context for a turn.

```
context <session_id> <turn_id>
```

Example:
```
context abc123 turn-456
```

## Approval Commands

### approvals

List pending approvals.

```
approvals
```

Options:
- `--session <id>` - Filter by session

Example:
```
approvals --session abc123
```

### approve

Approve a pending action.

```
approve <approval_id>
```

Example:
```
approve approval-xyz
```

### deny

Deny a pending action.

```
deny <approval_id> [reason]
```

Arguments:
- `approval_id` - The approval to deny
- `reason` - Optional reason for denial

Example:
```
deny approval-xyz Running tests is premature
```

## Tool Commands

### tools

List available tools.

```
tools
```

Shows all tools the agent can use with descriptions.

## Monitoring Commands

### health

Check server health.

```
health
```

Shows:
- Server status
- Version
- Uptime

### readiness

Check if server is ready.

```
readiness
```

Shows:
- Ready status
- Dependency status (database, etc.)

### metrics

View server metrics.

```
metrics
```

Shows:
- Active sessions
- Total turns
- Pending approvals
- Token usage

## Policy Commands

### policy

View current policy.

```
policy
```

Shows:
- Trust tier settings
- Provider configuration
- Retention class

### policy-route

View policy for specific parameters.

```
policy-route <trust_tier> <sensitivity> <task>
```

Arguments:
- `trust_tier` - tier0, tier1, tier2, tier3
- `sensitivity` - A, B, C, D
- `task` - analysis, codegen

Example:
```
policy-route tier1 B codegen
```

## Audit Commands

### event-types

List all event types.

```
event-types
```

### replay

Replay session events.

```
replay <session_id> [event_type]
```

Arguments:
- `session_id` - Session to replay
- `event_type` - Optional filter

Example:
```
replay abc123
replay abc123 tool_invocation
```

### diff

View changes from a session.

```
diff <session_id>
```

Shows file mutations made during the session.

### diff-rich

Render file mutations grouped by file/hunk with richer formatting.

```
diff-rich <session_id> [event_type]
```

Arguments:
- `session_id` - Session to inspect
- `event_type` - Optional event filter (for example `file_mutation`)

### diag

Fetch diagnostics for a file in a session repository.

```
diag <session_id> <path>
```

### sym

Fetch document symbols for a file.

```
sym <session_id> <path>
```

### hover

Fetch hover information at a file position.

```
hover <session_id> <path> <line> <col>
```

### def

Fetch definition locations at a file position.

```
def <session_id> <path> <line> <col>
```

LSP typed API failures are surfaced with explicit error types such as:
- `lsp_server_unavailable`
- `lsp_backend_unhealthy`
- `lsp_request_timeout`

## Utility Commands

### exit

Exit the TUI.

```
exit
```

Or press `Ctrl+C`.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SYNDICATE_CONTROLPLANE_URL` | Control plane URL | http://localhost:7777 |

## Command Aliases

| Command | Alias |
|---------|-------|
| start | new-session |
| ask | turn |
| approvals | approvals |
| sessions | sessions |
| tools | tools |
| health | health |
| replay | replay |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Ctrl+C` | Exit |
| `Ctrl+L` | Clear screen |
| `Tab` | Auto-complete |
