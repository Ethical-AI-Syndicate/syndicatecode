# AI Coding CLI – Approval State Machine

This document defines the lifecycle of actions requiring user approval.

Approval must bind to a specific action and its arguments.

---

# Approval Goals

- prevent accidental destructive actions
- provide clear operator visibility
- ensure deterministic decision logging

---

# Approval States

```
proposed
   ↓
pending
   ↓
approved / denied
   ↓
executed / cancelled
```

---

# State Definitions

## Proposed

The agent proposes a tool invocation that requires approval.

Metadata recorded:

- tool name
- arguments hash
- side effect classification
- affected paths

---

## Pending

Approval request is displayed to operator.

Operator sees:

- tool name
- arguments
- diff preview (if applicable)
- side effect category

---

## Approved

Operator authorizes the action.

The approval binds to:

- exact arguments
- tool name
- path scope

If arguments change, approval becomes invalid.

---

## Denied

Operator rejects the action.

Control plane records reason.

Agent must re-plan.

---

## Executed

Tool successfully executed after approval.

Event recorded in session log.

---

## Cancelled

Approval expired or session terminated.

---

# Approval Expiry

Approvals must expire if:

- arguments change
- session context changes significantly
- timeout threshold reached

---

# Example Approval Event

```json
{
  "approval_id": "uuid",
  "tool": "apply_patch",
  "arguments_hash": "sha256",
  "side_effect": "write",
  "paths": ["src/auth/service.go"],
  "state": "pending"
}
```

---

# Security Requirement

Approvals must be specific and immutable.

Vague approval prompts create false safety.

