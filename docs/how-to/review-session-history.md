# Review Session History

This guide explains how to replay and audit past sessions.

## Why review sessions

Session replay lets you:
- Understand what happened during a session
- Audit agent actions for security
- Debug issues or misunderstandings
- Review approval decisions

## Basic replay

To replay a session:

```
replay <session_id>
```

This returns all events in chronological order:
- Session start
- Tool invocations
- Approval decisions
- Model responses
- Turn completion

For mutation-focused review with grouped hunks and file headers:

```
diff-rich <session_id>
```

## Viewing session list

First, find the session you want to review:

```
sessions
```

This lists all sessions with:
- Session ID
- Repository path
- Trust tier
- Status (active, completed, terminated)
- Created and updated timestamps

## Filtering events

You can filter by event type:

```
replay <session_id> <event_type>
```

Available event types include:
- `tool_invocation` - Tool execution requests
- `tool_result` - Tool execution results
- `model_invocation` - AI model calls
- `approval_proposed` - Approval requests
- `approval_decided` - Approval decisions
- `file_mutation` - File changes
- `lsp_request` - LSP lookups invoked through TUI/control-plane

## Understanding event data

Each event includes:

```json
{
  "id": "event-id",
  "session_id": "session-id",
  "turn_id": "turn-id",
  "event_type": "tool_invocation",
  "timestamp": "2026-03-12T10:30:00Z",
  "tool_name": "read_file",
  "tool_input": { "path": "/repo/main.go" },
  "success": true,
  "duration_ms": 45
}
```

## Example: Finding a specific action

Suppose you want to find what changed a particular file:

1. List sessions:
   ```
   sessions
   ```

2. Replay the relevant session:
   ```
   replay <session_id>
   ```

3. Search for file mutations:
   ```
   replay <session_id> file_mutation
   ```

This shows all file changes in that session.

## Example: Reviewing approval decisions

To see all approval decisions:

```
replay <session_id> approval_decided
```

This shows:
- Each approval requested
- Whether it was approved or denied
- The reason (if provided)

## Using context inspector

For a specific turn, view exactly what context was sent to the AI:

```
context <session_id> <turn_id>
```

This shows:
- System prompt
- Repository context (files, structure)
- Conversation history
- Token counts

## Exporting for audit

To export a complete session for external audit:

```
export <session_id>
```

This generates a JSON file with:
- All events
- Full context at each turn
- Approval decisions
- File mutations

The export is automatically redacted for sensitive data.

## Best practices

### Regular reviews

Periodically review session history to:
- Verify agent behavior
- Understand decision-making patterns
- Identify areas for improvement

### Use for debugging

When something goes wrong:
1. Replay the session
2. Identify the problematic turn
3. Review the context that was provided
4. Understand what led to the issue

### Maintain audit trail

Session history is retained in the SQLite database. Configure retention settings in the server configuration.
