# AI Coding CLI – Control Plane API Specification

This document defines the core APIs exposed by the local control plane. The control plane is the authoritative component responsible for policy enforcement, session state, and tool orchestration.

All clients (TUI, IDE extensions, automation clients) communicate with the control plane through these APIs.

---

# Design Goals

- UI clients remain thin
- policy enforcement happens server-side
- all state transitions generate events
- deterministic behavior for audit and replay

---

# API Transport

Recommended implementation:

- Local HTTP server
- WebSocket stream for events
- JSON payloads

Example endpoint base:

```
http://localhost:7777/api/v1
```

---

# Session Management

## Create Session

```
POST /sessions
```

Response:

```json
{
  "session_id": "uuid",
  "repo_path": "/repo",
  "trust_tier": "tier1",
  "created_at": "timestamp"
}
```

---

## Get Session

```
GET /sessions/{session_id}
```

Returns session metadata and configuration.

---

## List Sessions

```
GET /sessions
```

Returns active and historical sessions.

---

# Turn Execution

## Submit User Turn

```
POST /sessions/{session_id}/turns
```

Payload:

```json
{
  "message": "Refactor the auth service",
  "files": ["src/auth/service.go"]
}
```

Control plane responsibilities:

- assemble context
- evaluate policy
- initiate agent runtime

---

# Tool Invocation

## Tool Request Event

Agent proposes a tool invocation.

Control plane evaluates:

- policy
- trust tier
- approval requirement

---

## Tool Execution

```
POST /tools/execute
```

Payload:

```json
{
  "tool": "read_file",
  "arguments": {
    "path": "src/auth/service.go"
  }
}
```

---

# Approval Workflow

## Pending Approvals

```
GET /approvals
```

Response includes pending decisions.

---

## Approve Action

```
POST /approvals/{approval_id}
```

Payload:

```json
{
  "decision": "approve"
}
```

Approval binds to:

- tool name
- arguments
- side effect classification

---

# Event Streaming

Clients subscribe via WebSocket.

Events include:

- session events
- tool invocation events
- approval requests
- model responses
- file mutations

Example event:

```json
{
  "event_type": "tool_invocation",
  "tool": "read_file",
  "arguments": {
    "path": "src/auth/service.go"
  }
}
```

---

# Context Inspection

## Inspect Turn Context

```
GET /sessions/{session_id}/turns/{turn_id}/context
```

Returns all context fragments included in the prompt envelope.

---

# Policy Inspection

## Active Policy

```
GET /policy
```

Returns policy configuration used for current session.

---

# Replay API

## Replay Session

```
GET /sessions/{session_id}/events
```

Returns ordered event stream for forensic replay.

---

# Error Handling

Errors must include:

- error type
- policy reason (if denied)
- retryability

Example:

```json
{
  "error": "policy_denied",
  "reason": "write denied for path",
  "retryable": false
}
```

