# Security Model

This document explains SyndicateCode's security architecture.

## Philosophy

Security through:
- **Isolation** - Tools run in constrained sandbox
- **Transparency** - All actions logged and auditable
- **Control** - User approval required for sensitive operations
- **Limits** - Trust tiers restrict capabilities

## Trust Tiers

Trust tiers define what the agent can do without explicit approval.

### Tier Comparison

| Capability | tier0 | tier1 | tier2 | tier3 |
|------------|-------|-------|-------|-------|
| Read files | ✓ | ✓ | ✓ | ✓ |
| Max loop depth | 1 | 3 | 5 | 10 |
| Auto-execute tools | Limited | Standard | Most | All |
| Output size | 64KB | 256KB | 512KB | 1MB |
| Timeout | 30s | 120s | 300s | 600s |

### Choosing a Tier

- **tier0** - Maximum control, exploration
- **tier1** - Default for most work
- **tier2** - Complex refactoring
- **tier3** - Trusted autonomous work

## Sandbox

### Command Restrictions

Tools execute in a sandbox with:

1. **Allowlisted commands** - Only approved commands can run
2. **Working directory limits** - Cannot access files outside repo
3. **Timeout limits** - Maximum execution time per command
4. **Output limits** - Maximum output bytes

### Tool Categories

#### Safe (Auto-approved)

- Read files
- Search code
- Git status
- List directory

#### Requires Approval

- Modify files
- Run shell commands
- Execute tests
- Create/delete files

### Execution Flow

```
Tool requested
     │
     ▼
Check: Allowed command?
     │── No ──▶ Reject
     │
     ▼
Check: Within working directory?
     │── No ──▶ Reject
     │
     ▼
Check: Requires approval?
     │── Yes ──▶ Request approval
     │
     ▼
Execute with limits:
- Timeout
- Output size
     │
     ▼
Return result or error
```

## Approval Workflow

### What Requires Approval

- File modifications
- Shell command execution
- External network calls
- Any state-changing operation

### Approval Lifecycle

```
Agent proposes action
     │
     ▼
User notified (TUI shows "awaiting_approval")
     │
     ▼
User reviews:
- Tool name
- Parameters
- Target files
- Reason
     │
     ▼
User decides:
- Approve: Execute tool
- Deny: Reject, agent tries alternative
```

### Approval Audit

Every approval decision is logged:
- Who decided
- When
- Reason (if provided)
- Result

## Secret Handling

### Detection

The system identifies:
- API keys
- Passwords
- Tokens
- Private keys
- Environment variables with secrets

### Redaction

Secrets are redacted from:
- Model prompts
- Export files
- Log outputs
- Event payloads

### Persistence

Sensitive data is:
- Never persisted to database
- Cleared after each turn
- Excluded from exports

## Audit Trail

### What's Logged

Every action generates an event:

- Session start/end
- Turn execution
- Tool invocation
- Tool result
- Approval request
- Approval decision
- File mutations
- Model calls

### Event Data

Each event includes:
- Timestamp
- Session/turn ID
- Actor (agent/user/system)
- Action details
- Result

### Retention

Event retention depends on retention class:

| Class | Use Case | Default Retention |
|-------|----------|-------------------|
| ephemeral | Quick tasks | 7 days |
| standard | Normal work | 30 days |
| persistent | Important sessions | Until deleted |

## Policy Engine

### Policy Components

1. **Trust tier rules** - What each tier allows
2. **Provider rules** - Which AI provider to use
3. **Sensitivity levels** - Data sensitivity classification
4. **Retention classes** - How long to keep data

### Policy Evaluation

When a request comes in:

```
Request: tool=apply_patch, tier=tier1
     │
     ▼
Check trust tier limits
     │
     ▼
Check tool permissions
     │
     ▼
Determine approval requirement
     │
     ▼
Select provider
     │
     ▼
Apply retention class
```

## Network Security

### Local Only

By default, SyndicateCode runs locally:
- No external network access for control plane
- Tools can make external calls (with approval)

### API Authentication

All API calls require:
- Bearer token authentication
- Token configured server-side

### Best Practices

1. **Use tier0 for unfamiliar repos**
2. **Review all approvals**
3. **Monitor session history**
4. **Configure API token in production**
5. **Use export redaction**

## Threat Model

### Protected Assets

- Repository source code
- Secrets in files
- Local filesystem
- Git credentials
- Session data

### Threat Actors

1. **Curious Operator** - May approve unsafe actions
2. **Malicious Repository** - Prompt injection in code
3. **Malicious Plugins** - MCP servers that exfiltrate
4. **Model Provider** - External data retention
5. **Compromised Dependencies** - Vulnerable libraries

### Mitigations

| Threat | Mitigation |
|--------|------------|
| Unsafe approvals | Review before approve |
| Prompt injection | Context validation |
| Malicious plugins | Plugin capability manifests |
| Provider risk | Secret redaction |
| Vulnerable deps | Dependency scanning |
