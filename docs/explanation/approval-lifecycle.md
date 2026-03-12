# Approval Lifecycle

This document explains how approvals flow through the system from proposal to execution.

## Overview

Approvals ensure the user maintains control over sensitive operations. The agent cannot modify files or run commands without explicit permission.

## Why Approvals Exist

Approvals protect against:
- **Unintended modifications** - Catch mistakes before they happen
- **Security risks** - Review shell commands before execution
- **Scope creep** - Agent stays on task
- **Audit trail** - Every change is authorized

## Approval States

Each approval moves through a defined lifecycle:

```
┌──────────┐     ┌─────────┐     ┌───────────┐
│ proposed │────▶│ pending │────▶│ approved │
└──────────┘     └─────────┘     └───────────┘
     │                │                 │
     │                │                 ▼
     │                │           ┌───────────┐
     │                └──────────▶│  denied   │
     │                             └───────────┘
     │                                   │
     │                                   ▼
     │                             ┌────────────┐
     └─────────────────────────────│ cancelled │
                                   └────────────┘
```

### State Descriptions

| State | Description | Next States |
|-------|-------------|-------------|
| `proposed` | Agent wants to run tool | `pending`, `cancelled` |
| `pending` | Awaiting user decision | `approved`, `denied`, `cancelled` |
| `approved` | User said yes | `executed`, `cancelled` |
| `denied` | User said no | terminal |
| `executed` | Tool completed | terminal |
| `cancelled` | Cancelled (agent or system) | terminal |

## Triggering Approval

### Automatic Triggers

Approval is automatically requested when:

1. **Tool type** - Writing to files, shell execution
2. **Trust tier** - Lower tiers require more approvals
3. **File scope** - Modifying multiple files
4. **External calls** - Network requests
5. **Policy rules** - Custom policy configurations

### Example Triggers

```
# File modification
Tool: apply_patch
Path: /repo/auth.go

# Shell execution
Tool: restricted_shell
Command: go test ./...

# Multiple file operations
Tool: apply_patch
Paths: [auth.go, login.go, config.go]
```

## Approval Request

### What's Included

When approval is requested, the user sees:

- **Tool name** - What's being executed
- **Parameters** - Input being passed
- **Target** - Files or resources affected
- **Reason** - Why the agent wants to do this
- **Context** - Relevant code or findings

### Example Request

```
Approval Required
─────────────────
Tool: apply_patch
File: auth.go
Operation: Add try-catch at line 42

Reason: To handle potential errors from parseConfig

[approve] [deny]
```

## User Decision

### Approve

When user approves:

1. Tool executes
2. Result returned to agent
3. Agent continues or completes
4. Decision logged to audit

### Deny

When user denies:

1. Tool does not execute
2. Agent notified of denial
3. Agent may:
   - Propose alternative approach
   - Ask for clarification
   - Give up on that task

### No Decision

If user doesn't respond:
- Turn may timeout
- Agent can retry within turn
- Approval remains pending

## Execution Flow

### Complete Flow

```
1. User sends request
        │
        ▼
2. Agent processes
        │
        ▼
3. Agent proposes tool call
        │
        ▼
4. System checks: approval needed?
        │
        ├──No──▶ Execute immediately
        │
        ▼
5. Create approval (state: proposed)
        │
        ▼
6. Notify user (state: pending)
        │
        ▼
7. User reviews and decides
        │
        ├──Approve──▶ state: approved
        │                │
        │                ▼
        │            Execute tool
        │                │
        │                ▼
        │            state: executed
        │                │
        │                ▼
        │            Return result
        │
        └──Deny──────▶ state: denied
                         │
                         ▼
                     Agent notified
```

### Concurrent Approvals

- Multiple approvals can be pending
- User can approve/deny in any order
- Agent waits for relevant approval before continuing

## Audit

### What's Logged

For each approval:

- `approval_proposed` - When created
- `approval_approved` or `approval_denied` - Decision
- `approval_executed` - If approved, result
- `approval_cancelled` - If cancelled

### Audit Fields

```json
{
  "approval_id": "uuid",
  "session_id": "uuid",
  "turn_id": "uuid",
  "tool_name": "apply_patch",
  "tool_input": { ... },
  "state": "approved",
  "decided_by": "user",
  "decided_at": "timestamp",
  "reason": "Safe to proceed"
}
```

## Integration with Turn Lifecycle

Approvals integrate with turn execution:

```
Turn: active
  │
  ├─▶ Agent thinks
  │
  ├─▶ Agent proposes tool
  │
  ├─▶ If approval needed:
  │     Turn state: awaiting_approval
  │     │
  │     ├─▶ Approved ──▶ Turn: active
  │     │
  │     └─▶ Denied ──▶ Agent reconsiders
  │
  ├─▶ Tool executes
  │
  └─▶ Turn: completed
```

## Best Practices

### For Users

1. **Review carefully** - Don't approve blindly
2. **Check file paths** - Verify correct files
3. **Understand the operation** - Know what will happen
4. **Start conservative** - Use tier0 until familiar

### For Agents

1. **Explain clearly** - Why approval is needed
2. **Be specific** - Exact file locations, line numbers
3. **Break down** - Large changes into smaller approvals
4. **Provide context** - Relevant code snippets

## Configuration

### Per-Tool Settings

Tools can be configured to always require approval:

```json
{
  "tools": {
    "restricted_shell": {
      "always_require_approval": true
    },
    "read_file": {
      "always_require_approval": false
    }
  }
}
```

### Trust Tier Impact

Lower trust tiers require more approvals:

- **tier0** - Most operations require approval
- **tier1** - Standard operations need approval
- **tier2** - Fewer approvals needed
- **tier3** - Most operations auto-approved
