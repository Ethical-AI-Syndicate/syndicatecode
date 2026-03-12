# Export Session Data

This guide explains how to safely export session data for audit, debugging, or backup.

## Why export sessions

Exports provide:
- **Audit records** - Complete history for compliance
- **Debugging** - Analyze issues offline
- **Migration** - Move sessions to another system
- **Backup** - Preserve session data

## Basic export

To export a session:

```
export <session_id>
```

This returns a JSON document containing:
- Session metadata
- All turns
- All events
- Context at each turn

## Export format

The export includes:

```json
{
  "session": {
    "id": "session-123",
    "repo_path": "/repo",
    "trust_tier": "tier1",
    "status": "completed",
    "created_at": "2026-03-12T10:00:00Z",
    "updated_at": "2026-03-12T10:45:00Z"
  },
  "turns": [
    {
      "id": "turn-456",
      "message": "Fix the bug",
      "status": "completed",
      "context": { ... },
      "events": [ ... ]
    }
  ],
  "events": [
    {
      "id": "event-789",
      "type": "tool_invocation",
      "timestamp": "2026-03-12T10:05:00Z",
      "data": { ... }
    }
  ]
}
```

## Export options

### Include artifacts

By default, exports include artifact references. To exclude:

```
export <session_id>?include_artifacts=false
```

### Filter by date range

Export only events within a time window:

```
export <session_id>?start=2026-03-12T10:00:00Z&end=2026-03-12T11:00:00Z
```

## Automatic redaction

All exports are automatically redacted for security:

- **Secrets** - API keys, passwords, tokens are removed
- **Sensitive paths** - Paths outside the repository are excluded
- **Model prompts** - Sensitive context is filtered

This ensures exports don't leak sensitive information.

## Retention classes

Exports respect the session's retention class:

| Class | Description | Retention |
|-------|-------------|-----------|
| ephemeral | Short-lived sessions | 7 days |
| standard | Normal sessions | 30 days |
| persistent | Important sessions | Until manually deleted |

Configure retention in the provider policy.

## Example: Full audit export

For complete audit purposes:

```bash
# Get session list
sessions

# Export with all artifacts
export session-123 > audit-session-123.json

# Include all events
export session-123?include_events=true > full-audit.json
```

## Example: Debug export

For debugging a specific issue:

```bash
# Export recent turns only
export session-123?turns=recent > debug.json
```

## Storage

Exported files are saved to:
- Local filesystem (default)
- Custom path configurable in server

The export includes a manifest of all included data for verification.

## Best practices

### Regular exports

Export important sessions periodically:
- After significant work completes
- Before session deletion
- For compliance requirements

### Verify exports

After export, verify integrity:
```bash
# Check export is valid JSON
cat export.json | jq .

# Count events
cat export.json | jq '.events | length'
```

### Secure storage

Treat exports as sensitive:
- Encrypt at rest
- Limit access
- Delete when no longer needed
