# AI Coding CLI – Event Schema Specification

This document defines the canonical event model used by the system. All state transitions must produce events. Events provide replayability, auditability, and debugging capability.

---

# Event Model Principles

- append-only
- timestamped
- immutable
- replayable

Events are stored in chronological order.

---

# Core Event Structure

```json
{
  "event_id": "uuid",
  "session_id": "uuid",
  "turn_id": "uuid",
  "timestamp": "RFC3339",
  "event_type": "string",
  "actor": "agent|user|system|plugin",
  "policy_version": "string",
  "trust_tier": "tier0|tier1|tier2|tier3"
}
```

---

# Event Types

## Session Started

```json
{
  "event_type": "session_started",
  "repo_path": "/repo",
  "user": "operator"
}
```

---

## Turn Started

```json
{
  "event_type": "turn_started",
  "message": "Refactor auth service"
}
```

---

## Context Fragment Included

```json
{
  "event_type": "context_fragment",
  "source_type": "file|tool_output|instruction",
  "source_ref": "src/auth/service.go",
  "token_count": 412,
  "truncated": false
}
```

---

## Model Invocation

```json
{
  "event_type": "model_invocation",
  "provider": "openai",
  "model": "gpt-4",
  "input_tokens": 1400
}
```

---

## Tool Invocation

```json
{
  "event_type": "tool_invocation",
  "tool": "read_file",
  "arguments_hash": "sha256"
}
```

---

## Tool Result

```json
{
  "event_type": "tool_result",
  "tool": "read_file",
  "result_size": 1200
}
```

---

## Approval Requested

```json
{
  "event_type": "approval_requested",
  "approval_id": "uuid",
  "tool": "apply_patch",
  "side_effect": "write"
}
```

---

## Approval Decision

```json
{
  "event_type": "approval_decision",
  "decision": "approved"
}
```

---

## File Mutation

```json
{
  "event_type": "file_mutation",
  "path": "src/auth/service.go",
  "before_hash": "sha256",
  "after_hash": "sha256"
}
```

---

## Turn Completed

```json
{
  "event_type": "turn_completed"
}
```

---

# Event Storage

Recommended schema:

- events table
- indexed by session_id
- indexed by timestamp

This enables replay and debugging.

---

# Replay Rules

Replay must reconstruct:

- context assembly
- model calls
- tool executions
- approvals
- file mutations

This ensures deterministic debugging.

