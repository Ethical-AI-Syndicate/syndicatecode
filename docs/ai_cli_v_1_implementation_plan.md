# AI Coding CLI – V1 Implementation Plan

This document defines a phased approach to building a defensible AI coding CLI.

---

# Phase 1 – Control Plane Skeleton

Goal: inspectability before mutation.

Implement:

- local control plane server
- SQLite event store
- TUI client
- session management
- read‑only tools
- context manifest per turn
- deterministic token budgeting
- trust‑tier configuration

Deliverable:

Operator can inspect exactly what context is sent to the model.

---

# Phase 2 – Safe Editing

Goal: controlled repository modification.

Implement:

- patch proposal workflow
- diff viewer
- approval UI
- file hash tracking
- syntax validation
- formatting validation
- rollback checkpoints

Deliverable:

All file edits are patch‑based and attributable.

---

# Phase 3 – Constrained Execution

Goal: bounded tool execution.

Implement:

- structured test tools
- lint tools
- restricted shell tool
- environment filtering
- working directory enforcement
- tool timeouts
- tool output limits

Deliverable:

Agent execution surface is controlled and observable.

---

# Phase 4 – Secret Handling

Goal: prevent sensitive data exposure.

Implement:

- secret detection pipeline
- prompt redaction
- persistence redaction
- export redaction
- sensitive session mode

Deliverable:

Sensitive data cannot accidentally leak to external model providers.

---

# Phase 5 – Extension Model

Goal: extensibility without uncontrolled access.

Implement:

- plugin capability manifests
- plugin trust levels
- MCP integration
- plugin event logging

Deliverable:

Plugins are governed by explicit permissions.

---

# Features to Defer From V1

These features expand complexity and blast radius:

- multi‑agent orchestration
- autonomous background loops
- generalized vector retrieval
- cloud sync by default
- open plugin marketplace
- repository‑wide autonomous refactors
- multi‑user RBAC

These can be added once the control plane is stable.

---

# Success Criteria

The system is ready for initial release when:

1. Operators can inspect model context.
2. File changes are replayable.
3. Policies are enforced below the model layer.
4. Secrets are filtered before model egress.
5. Shell execution is constrained.
6. Trust tiers alter system behavior.
7. Approvals bind to exact actions.
8. Session exports are safe.

---

# Guiding Principle

The UI is not the product.

The product is:

- the control plane
- the provenance model
- the safety boundaries

Everything else is an interface to those systems.

