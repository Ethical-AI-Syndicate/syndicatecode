# Configuration Reference

Complete reference for SyndicateCode configuration options.

## Server Configuration

### Environment Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `ADDR` | string | `:7777` | Server listen address |
| `DBPATH` | string | `syndicatecode.db` | SQLite database path |
| `APITOKEN` | string | (none) | Authentication token |
| `READ_TIMEOUT` | duration | `30s` | HTTP read timeout |
| `WRITE_TIMEOUT` | duration | `30s` | HTTP write timeout |
| `PROVIDER_POLICY_PATH` | string | (none) | Path to provider policy JSON |

### Programmatic Config

```go
type Config struct {
    Addr               string        // Server address (default: ":7777")
    DBPath             string        // Database path (default: "syndicatecode.db")
    APIToken           string        // Authentication token
    ReadTimeout        time.Duration // HTTP read timeout (default: 30s)
    WriteTimeout       time.Duration // HTTP write timeout (default: 30s)
    ProviderPolicyPath string        // Path to provider policy JSON
}
```

### Default Values

```go
func DefaultConfig() *Config {
    return &Config{
        Addr:               ":7777",
        DBPath:             "syndicatecode.db",
        ReadTimeout:        30 * time.Second,
        WriteTimeout:       30 * time.Second,
        ProviderPolicyPath: "",
    }
}
```

## Provider Policy

Provider policy is defined in JSON and loaded at startup.

### Policy File Structure

```json
{
  "providers": [
    {
      "name": "anthropic-default",
      "trust_tiers": ["tier0", "tier1", "tier2", "tier3"],
      "sensitivity": ["A", "B", "C", "D"],
      "tasks": ["analysis", "codegen"],
      "retention_class": "ephemeral",
      "fallback_eligible": true
    }
  ]
}
```

### Provider Rule Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Provider identifier |
| `trust_tiers` | array | Applicable trust tiers |
| `sensitivity` | array | Sensitivity levels |
| `tasks` | array | Task types |
| `retention_class` | string | Data retention class |
| `fallback_eligible` | bool | Can be fallback provider |

## Trust Tier Configuration

Trust tiers are defined in `internal/trust/policy.go`.

### Default Limits

| Tier | Max Output | Timeout | Max Depth |
|------|------------|---------|-----------|
| tier0 | 64 KB | 30s | 1 |
| tier1 | 256 KB | 120s | 3 |
| tier2 | 512 KB | 300s | 5 |
| tier3 | 1 MB | 600s | 10 |

### Customizing Trust Tiers

Modify `DefaultConfig()` in `internal/trust/policy.go`:

```go
func DefaultConfig(trustTier string) Config {
    p := trust.DefaultPolicy()
    maxOutput := 1024 * 1024
    turnTimeout := 600 * time.Second
    
    switch trustTier {
    case "tier0":
        maxOutput = 64 * 1024
        turnTimeout = 30 * time.Second
    case "tier1":
        maxOutput = 256 * 1024
        turnTimeout = 120 * time.Second
    // ... etc
    }
    
    return Config{
        MaxDepth: p.MaxLoopDepth(trustTier),
        MaxToolCalls: p.MaxToolCalls(trustTier),
        MaxRetries: maxRetries,
        TurnTimeout: turnTimeout,
        MaxOutputBytes: maxOutput,
    }
}
```

## Database Configuration

### SQLite Settings

The database uses WAL mode for concurrency:

```
sqlite3 "syndicatecode.db?_journal_mode=WAL&_busy_timeout=5000"
```

### Schema

Database schema is managed through migrations in `internal/audit/migrations.go`.

### Tables

| Table | Purpose |
|-------|---------|
| `events` | All audit events |
| `sessions` | Session metadata |
| `turns` | Turn data |
| `tool_invocations` | Tool execution records |
| `model_invocations` | Model call records |
| `file_mutations` | File change records |
| `artifacts` | Artifact references |

## Security Configuration

### API Token

Set a secure token:

```bash
export APITOKEN=your-secure-token-here
```

Clients must include:
```
Authorization: Bearer your-secure-token-here
```

### Allowed Commands

Sandbox execution restricts commands. Configure in `internal/sandbox/`:

```go
type Config struct {
    RepoRoot       string
    AllowedCmds    map[string]struct{}
    DefaultTimeout time.Duration
    MaxOutputBytes int
}
```

### Working Directory Restrictions

Commands can only execute within the configured repository root.

## Logging

### Log Levels

Configure via standard Go `log` package.

### What Gets Logged

- Server start/stop
- Session lifecycle
- Approval decisions
- Policy enforcement
- Errors and warnings

### What Doesn't Get Logged

- Tool inputs/outputs (stored in audit events)
- API request bodies (may contain secrets)
- Model prompts (stored in events)

## Environment Examples

### Development

```bash
export ADDR=:7777
export DBPATH=syndicatecode.db
go run ./cmd/server
```

### Production

```bash
export ADDR=:443
export DBPATH=/var/lib/syndicatecode/syndicatecode.db
export APITOKEN=production-token
export PROVIDER_POLICY_PATH=/etc/syndicatecode/policy.json
go run ./cmd/server
```

### Docker

```bash
docker run -p 7777:7777 \
  -v /data:/data \
  -e DBPATH=/data/syndicatecode.db \
  -e APITOKEN=token \
  syndicatecode
```
