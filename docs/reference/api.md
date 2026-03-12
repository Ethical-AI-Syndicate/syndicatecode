# Control Plane API Reference

Complete reference for the SyndicateCode Control Plane REST API.

## Base URL

```
http://localhost:7777/api/v1
```

## Authentication

All endpoints require a Bearer token in the Authorization header:

```
Authorization: Bearer <token>
```

Configure the token via the `APITOKEN` environment variable or server config.

## Endpoints

### Sessions

#### Create Session

```
POST /sessions
```

Request:
```json
{
  "repo_path": "/path/to/repo",
  "trust_tier": "tier1"
}
```

Response:
```json
{
  "session_id": "uuid",
  "repo_path": "/path/to/repo",
  "trust_tier": "tier1",
  "status": "active",
  "created_at": "2026-03-12T10:00:00Z"
}
```

#### List Sessions

```
GET /sessions
```

Query parameters:
- `status` - Filter by status (active, completed, terminated)

Response:
```json
{
  "sessions": [
    {
      "session_id": "uuid",
      "repo_path": "/path/to/repo",
      "trust_tier": "tier1",
      "status": "active",
      "created_at": "2026-03-12T10:00:00Z"
    }
  ]
}
```

#### Get Session

```
GET /sessions/{session_id}
```

Response:
```json
{
  "session_id": "uuid",
  "repo_path": "/path/to/repo",
  "trust_tier": "tier1",
  "status": "active",
  "created_at": "2026-03-12T10:00:00Z",
  "updated_at": "2026-03-12T10:30:00Z"
}
```

### Turns

#### Create Turn

```
POST /sessions/{session_id}/turns
```

Request:
```json
{
  "message": "Fix the bug in login",
  "files": ["auth.go", "login.go"]
}
```

Response:
```json
{
  "turn_id": "uuid",
  "session_id": "uuid",
  "status": "completed",
  "created_at": "2026-03-12T10:00:00Z"
}
```

WebSocket stream provides real-time events during turn execution.

#### Get Turn Context

```
GET /sessions/{session_id}/turns/{turn_id}/context
```

Response:
```json
{
  "turn_id": "uuid",
  "system_prompt": "...",
  "repository_context": { ... },
  "conversation_history": [ ... ],
  "token_counts": {
    "system": 4000,
    "repository": 45000,
    "history": 8000,
    "total": 57000
  }
}
```

### Tools

#### List Tools

```
GET /tools
```

Response:
```json
{
  "tools": [
    {
      "name": "read_file",
      "description": "Read a file from the repository",
      "input_schema": {
        "type": "object",
        "properties": {
          "path": { "type": "string" }
        },
        "required": ["path"]
      }
    }
  ]
}
```

### Approvals

#### List Approvals

```
GET /approvals
```

Query parameters:
- `session_id` - Filter by session

Response:
```json
{
  "approvals": [
    {
      "approval_id": "uuid",
      "session_id": "uuid",
      "turn_id": "uuid",
      "tool_name": "apply_patch",
      "state": "pending",
      "created_at": "2026-03-12T10:00:00Z"
    }
  ]
}
```

#### Decide Approval

```
POST /approvals/{approval_id}
```

Request:
```json
{
  "decision": "approve",
  "reason": "This is a safe change"
}
```

Response:
```json
{
  "approval_id": "uuid",
  "state": "approved",
  "decided_at": "2026-03-12T10:05:00Z"
}
```

### Policy

#### Get Policy

```
GET /policy
```

Response:
```json
{
  "version": "1.0.0",
  "trust_tiers": [ ... ],
  "providers": [ ... ],
  "retention_class": "standard"
}
```

#### Get Policy Route

```
GET /policy?trust_tier=tier1&sensitivity=B&task=codegen
```

Response:
```json
{
  "provider": "anthropic",
  "model": "claude-3-5-sonnet-20241022",
  "retention_class": "ephemeral"
}
```

### Events

#### Get Event Types

```
GET /events/types
```

Response:
```json
{
  "event_types": [
    "session_started",
    "turn_started",
    "tool_invocation",
    "approval_proposed",
    "approval_decided",
    "file_mutation"
  ]
}
```

#### Replay Session Events

```
GET /sessions/{session_id}/events
```

Query parameters:
- `event_type` - Filter by type

Response:
```json
{
  "events": [
    {
      "id": "uuid",
      "session_id": "uuid",
      "turn_id": "uuid",
      "event_type": "tool_invocation",
      "timestamp": "2026-03-12T10:00:00Z",
      "data": { ... }
    }
  ]
}
```

### Health

#### Health Check

```
GET /health
```

Response:
```json
{
  "status": "healthy",
  "version": "0.9.0",
  "uptime": "1h30m"
}
```

#### Readiness Check

```
GET /readiness
```

Response:
```json
{
  "ready": true,
  "dependencies": {
    "database": "ok"
  }
}
```

### Metrics

#### Get Metrics

```
GET /metrics
```

Response:
```json
{
  "sessions_active": 2,
  "turns_total": 150,
  "approvals_pending": 1,
  "tokens_used": 2500000
}
```

### Export

#### Export Session

```
GET /sessions/{session_id}/export
```

Query parameters:
- `include_artifacts` - Include artifact references (default: true)

Response: JSON document with full session data

## WebSocket Events

Connect to `/api/v1/sessions/{session_id}/events` for real-time event stream.

Event types:
- `text_delta` - Streaming response text
- `tool_use_start` - Tool execution started
- `tool_input_delta` - Tool input being built
- `turn_completed` - Turn finished
- `turn_failed` - Turn error
- `awaiting_approval` - Approval required

## Error Responses

All errors return:

```json
{
  "error": "error_type",
  "reason": "human readable reason",
  "retryable": false
}
```

Common error types:
- `not_found` - Resource doesn't exist
- `unauthorized` - Authentication required
- `policy_denied` - Action blocked by policy
- `invalid_request` - Malformed request
- `server_error` - Internal error
