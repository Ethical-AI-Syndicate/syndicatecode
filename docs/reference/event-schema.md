# Event Schema Reference

Complete reference for all event types in SyndicateCode.

## Overview

Events are the atomic unit of record in SyndicateCode. Every action generates an event stored in the audit system.

## Event Structure

All events follow this structure:

```json
{
  "id": "uuid",
  "session_id": "uuid",
  "turn_id": "uuid",
  "event_type": "event_type_name",
  "timestamp": "2026-03-12T10:00:00Z",
  "actor": "agent|user|system",
  "trust_tier": "tier1",
  "policy_version": "1.0.0",
  "payload": { ... }
}
```

## Event Types

### Session Events

#### session_started

Triggered when a new session is created.

```json
{
  "event_type": "session_started",
  "payload": {
    "repo_path": "/repo",
    "trust_tier": "tier1",
    "initial_context_size": 45000
  }
}
```

#### session_completed

Triggered when session ends normally.

```json
{
  "event_type": "session_completed",
  "payload": {
    "turns_count": 15,
    "total_tokens": 250000,
    "duration_seconds": 3600
  }
}
```

#### session_terminated

Triggered when session is forcefully ended.

```json
{
  "event_type": "session_terminated",
  "payload": {
    "reason": "user_requested",
    "turns_count": 8
  }
}
```

### Turn Events

#### turn_started

Triggered when a new turn begins.

```json
{
  "event_type": "turn_started",
  "payload": {
    "message": "Fix the login bug",
    "budget_allocated": 64000
  }
}
```

#### turn_completed

Triggered when turn finishes.

```json
{
  "event_type": "turn_completed",
  "payload": {
    "tool_calls_count": 5,
    "tokens_used": 42000,
    "duration_ms": 45000
  }
}
```

#### turn_failed

Triggered when turn errors.

```json
{
  "event_type": "turn_failed",
  "payload": {
    "error": "timeout",
    "error_message": "Turn exceeded 120s timeout"
  }
}
```

### Tool Events

#### tool_invocation

Triggered when a tool is invoked.

```json
{
  "event_type": "tool_invocation",
  "payload": {
    "tool_name": "read_file",
    "tool_id": "tool-uuid",
    "input": {
      "path": "/repo/auth.go"
    },
    "requires_approval": false
  }
}
```

#### tool_result

Triggered when tool completes.

```json
{
  "event_type": "tool_result",
  "payload": {
    "tool_id": "tool-uuid",
    "tool_name": "read_file",
    "success": true,
    "output": "...",
    "duration_ms": 45,
    "output_bytes": 2048
  }
}
```

#### tool_error

Triggered when tool fails.

```json
{
  "event_type": "tool_error",
  "payload": {
    "tool_id": "tool-uuid",
    "tool_name": "apply_patch",
    "error": "validation_failed",
    "error_message": "Patch could not be applied"
  }
}
```

### Approval Events

#### approval_proposed

Triggered when agent requests approval.

```json
{
  "event_type": "approval_proposed",
  "payload": {
    "approval_id": "uuid",
    "tool_name": "apply_patch",
    "tool_input": {
      "path": "/repo/auth.go",
      "patch": "..."
    },
    "reason": "File modification"
  }
}
```

#### approval_approved

Triggered when approval is granted.

```json
{
  "event_type": "approval_approved",
  "payload": {
    "approval_id": "uuid",
    "decided_by": "user",
    "decided_at": "2026-03-12T10:05:00Z"
  }
}
```

#### approval_denied

Triggered when approval is denied.

```json
{
  "event_type": "approval_denied",
  "payload": {
    "approval_id": "uuid",
    "decided_by": "user",
    "reason": "Not safe to modify",
    "decided_at": "2026-03-12T10:05:00Z"
  }
}
```

#### approval_executed

Triggered after approved tool completes.

```json
{
  "event_type": "approval_executed",
  "payload": {
    "approval_id": "uuid",
    "tool_result": "success"
  }
}
```

### Model Events

#### model_invocation

Triggered when AI model is called.

```json
{
  "event_type": "model_invocation",
  "payload": {
    "provider": "anthropic",
    "model": "claude-3-5-sonnet-20241022",
    "input_tokens": 35000,
    "output_tokens": 8000,
    "duration_ms": 2000
  }
}
```

### File Events

#### file_mutation

Triggered when a file is modified.

```json
{
  "event_type": "file_mutation",
  "payload": {
    "operation": "create|modify|delete",
    "path": "/repo/auth.go",
    "sha_before": "abc123",
    "sha_after": "def456",
    "approved": true
  }
}
```

#### file_read

Triggered when a file is read.

```json
{
  "event_type": "file_read",
  "payload": {
    "path": "/repo/main.go",
    "bytes_read": 4096,
    "context_included": true
  }
}
```

### Context Events

#### context_assembled

Triggered when context is assembled for a turn.

```json
{
  "event_type": "context_assembled",
  "payload": {
    "system_prompt_tokens": 4000,
    "repository_tokens": 45000,
    "history_tokens": 8000,
    "files_included": 12,
    "budget_total": 64000
  }
}
```

## Common Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique event identifier |
| `session_id` | string | Session this event belongs to |
| `turn_id` | string | Turn this event belongs to (if applicable) |
| `event_type` | string | Type of event |
| `timestamp` | ISO8601 | When event occurred |
| `actor` | string | Who initiated (agent, user, system) |
| `trust_tier` | string | Trust tier at time of event |
| `policy_version` | string | Policy version active |

## Retention

Events are retained based on session retention class:

| Class | Default Retention |
|-------|------------------|
| ephemeral | 7 days |
| standard | 30 days |
| persistent | Until manually deleted |
