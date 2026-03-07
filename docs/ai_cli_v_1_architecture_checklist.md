# AI Coding CLI – V1 Architecture Checklist

## System Boundary

```
TUI Client
  ↓
Local Control Plane
  ↓
Agent Runtime
  ↓
Tool Runners
  ↓
Local Repository / OS / Optional External Services
```

The **control plane is the authoritative component**. The UI is only a client.

---

# Core Components

## TUI Client
Responsibilities:

- render conversation
- render tool execution trace
- show approval prompts
- display diffs
- display context inspector
- session replay viewer

Non‑responsibilities:

- executing tools
- file mutation
- policy enforcement
- prompt construction

---

## Local Control Plane

Responsibilities:

- session lifecycle
- context assembly
- prompt generation
- token budgeting
- approval workflow
- policy enforcement
- secret filtering
- trust‑tier evaluation
- event logging
- tool authorization

This layer is the **system of record**.

---

## Agent Runtime

Responsibilities:

- planning loop
- tool selection
- model invocation
- observation / action cycle
- retry logic
- stop conditions

Non‑responsibilities:

- policy enforcement
- persistence
- OS boundary access

---

## Tool Runner Layer

Responsibilities:

- executing structured tools
- enforcing timeouts
- normalizing outputs
- isolating environment
- classifying side effects

Example tools:

- read_file
- search_code
- git_status
- run_tests
- apply_patch
- restricted_shell

---

## Persistence Layer

Initial implementation:

- SQLite

Stores:

- sessions
- events
- approvals
- context manifests
- tool results metadata
- file hashes
- policy bundles
- trust tier configuration

---

# Repository Layout (Suggested)

```
/cmd/cli
/cmd/server

/internal/controlplane
/internal/session
/internal/agent
/internal/context
/internal/policy
/internal/tools
/internal/sandbox
/internal/audit
/internal/secrets
/internal/models
/internal/git
/internal/patch
/internal/validation
/internal/trust
/internal/mcp

/pkg/tui
/pkg/api
/pkg/types
```

---

# Non‑Negotiable Controls

## Session & Eventing

- stable session ID
- stable turn ID
- stable tool invocation ID
- stable approval decision ID
- model identity recorded
- timestamped events
- deterministic event ordering
- crash‑safe persistence

---

## Context Provenance

Every context fragment must record:

- source type
- source location
- retrieval time
- token size
- inclusion reason
- truncation state

Operators must be able to inspect the **exact prompt envelope**.

---

## Policy Enforcement

Policy must be enforced in the control plane.

Capabilities must be separated:

- read
- write
- execute
- network

Policies should include:

- path rules
- network rules
- approval rules
- trust‑tier overrides

---

## Edit Safety

Default behavior:

- patch‑based edits

Required protections:

- preimage hash verification
- syntax validation
- formatting validation
- rollback checkpoints
- diff preview for high‑risk changes

---

## Tool Governance

Structured tools should be preferred.

Shell execution must be constrained:

- command allowlist
- working directory enforcement
- environment filtering
- timeout limits

---

## Secret Handling

Secrets must be filtered before:

- model prompt inclusion
- persistence
- export

Secret detection must apply to:

- `.env` files
- credential patterns
- tokens
- private keys

---

## Replayability

The system must support:

- complete event replay
- tool invocation reproduction
- approval reconstruction
- file change verification

---

## Reliability Limits

Required guardrails:

- max iterations per turn
- max tool calls per turn
- max model retries
- max tool output size
- token budget enforcement

---

# Trust Tier Model

## Tier 0 – Untrusted External

Default posture:

- read allowed
- write restricted
- shell restricted
- network denied

---

## Tier 1 – Internal Low Risk

- patch edits allowed
- tests allowed
- moderate approvals

---

## Tier 2 – Production Adjacent

- stricter approvals
- sensitive path controls
- branch isolation encouraged

---

## Tier 3 – Restricted / Regulated

- plugin restrictions
- local models only or approved vendors
- strict approvals
- full audit retention

---

# V1 Go / No‑Go Criteria

The product is not ready until:

1. Context used for a turn is inspectable
2. File changes are replayable
3. Policy enforcement exists below model layer
4. Secrets are filtered before model egress
5. Shell execution is constrained
6. Trust tiers alter behavior
7. Approvals bind to exact actions
8. Session export is safe

