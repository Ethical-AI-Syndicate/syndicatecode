# Configure Trust Tiers

This guide explains how to configure trust tiers to control what the AI agent can do.

## Understanding trust tiers

Trust tiers define resource limits and permissions for a session. Each tier controls:

- **Maximum output size** - How much data the agent can process
- **Timeout** - How long a single turn can run
- **Loop depth** - How many tool calls in a single response
- **Tool permissions** - Which tools require approval

## Default tier configuration

| Tier | Output Limit | Timeout | Max Depth | Best For |
|------|--------------|---------|-----------|----------|
| tier0 | 64 KB | 30s | 1 | Exploration, reading |
| tier1 | 256 KB | 120s | 3 | Standard development |
| tier2 | 512 KB | 300s | 5 | Complex refactoring |
| tier3 | 1 MB | 600s | 10 | Full autonomy |

## Starting a session with a specific tier

When creating a session, specify the tier:

```
start /path/to/repo tier1
```

Choose based on your needs:
- **tier0**: When you want maximum control, only read operations
- **tier1**: Default for most development tasks
- **tier2**: For larger refactoring tasks
- **tier3**: When you need the agent to work independently

## Customizing trust tiers

The trust tier behavior is defined in `internal/trust/policy.go`. You can modify:

1. **Output limits** - Maximum bytes per response
2. **Timeout duration** - Maximum time per turn
3. **Tool call limits** - Maximum tools per turn

### Example: Understanding tier behavior

When you start with tier0:
- The agent can read files
- Single tool calls execute automatically
- Larger operations require approval

When you start with tier3:
- The agent can execute multiple tools in sequence
- Longer running operations are allowed
- Fewer approval gates

## Best practices

### Start conservative

Begin with `tier0` or `tier1` until you understand what the agent will do. Escalate only when confident.

### Use tier0 for unfamiliar code

When working with unfamiliar repositories, start conservative to review each action.

### Reserve tier3 for trusted operations

Only use tier3 when:
- You understand the codebase well
- The task is well-defined
- You've verified the agent's behavior in lower tiers

## Checking current policy

View the active policy in your session:

```
policy
```

This shows:
- Current trust tier settings
- Allowed providers
- Retention class
- Sensitivity levels
