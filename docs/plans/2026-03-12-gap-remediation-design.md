# Gap Remediation Design (Master Baseline)

Date: 2026-03-12
Scope baseline: `master`
Compatibility posture: docs-first exactness
Delivery strategy: risk-first phases

## Objective

Fully close architecture/documentation gaps identified against:

- `docs/ai_cli_v_1_architecture_checklist.md`
- `docs/ai_cli_v_1_implementation_plan.md`
- `docs/ai_cli_control_plane_api_spec.md`

with priority on V1 Go/No-Go controls: inspectable context, safe edits, policy-below-model enforcement, pre-egress secret filtering, constrained shell execution, trust-tier behavior, approval binding, and safe session export.

## Approved Approach

Risk-first phased remediation.

1. Close safety-critical Go/No-Go gaps first.
2. Complete provenance and replay fidelity.
3. Harden contracts and policy parity.

## Section 1: Target Architecture

- Control plane is the authoritative orchestration path for turn execution and safety controls.
- Agent runtime consumes an already-assembled prompt envelope; it does not accept raw unvalidated turn input.
- Edit mutation flows through explicit safety gates before filesystem changes.
- Audit/provenance records are first-class and replay-verifiable.
- API contracts are aligned to docs-first response schemas.

## Section 2: Components and Data Flow

### Phase 1: Go/No-Go Safety Closure

- Introduce a control-plane turn execution orchestrator that performs:
  - session/turn resolution,
  - context fragment retrieval and ranking,
  - destination-aware redaction for model egress,
  - context manifest persistence,
  - model invocation with envelope-derived content.
- Remove raw-message model egress path.
- Introduce patch safety service wrapping parse/validate/checkpoint/apply/post-verify.

### Phase 2: Provenance and Replay Completeness

- Ensure tool/model/mutation records include session/turn/approval causality fields.
- Ensure file mutation events contain verification payload (path/type/pre/post hash/proposal reference).
- Ensure replay/export reconstructs both action history and policy/approval rationale.

### Phase 3: Contract and Policy Hardening

- Align endpoint responses with docs-first contract definitions.
- Enforce parity between core and plugin data-access/network policy controls.
- Normalize trust-tier/session input validation and error-envelope typing.

## Section 3: Error Handling, Rollback, and Invariants

### Required Invariants

- No model invocation without manifest record for the turn.
- No patch apply without successful preimage validation.
- No approval execution when computed call hash differs from approved hash.
- No replay filter acceptance for unknown event types.

### Failure Policies

- Context assembly/redaction failure: fail turn with causal event.
- Patch pre-check failure: fail fast with zero mutation.
- Post-checkpoint apply failure: restore checkpoint and emit rollback event.
- Audit persistence failure on mutation paths: hard fail.

### Rollback Semantics

- Checkpoint reference required for write-class mutations.
- Rollback events include checkpoint ref, affected paths, and restoration status.
- Replay/export shows rollback chain explicitly.

## Section 4: Testing Strategy and Acceptance Gates

### Test Strategy

- Unit tests for orchestrator, redaction destination routing, patch safety gates.
- Integration tests for end-to-end turn lifecycle with event assertions.
- Contract tests for all public response schemas and TUI client decode behavior.
- Regression tests mapped to each previously identified architecture gap.

### Acceptance Gates

- `go test ./...` passes.
- All Go/No-Go checklist controls mapped to executable tests.
- No critical gap remains open in architecture checklist mapping.
- Replay/export samples are causally reconstructible without manual inference.

## Gap-to-Phase Mapping

1. Context inspectability not wired end-to-end -> Phase 1
2. Secret filtering before model egress incomplete -> Phase 1
3. Safe editing controls partial (validator/checkpoint/syntax-format gates) -> Phase 1
4. File mutation replay provenance incomplete -> Phase 2
5. Tool invocation reproducibility metadata incomplete -> Phase 2
6. Session metadata/trust-tier validation drift -> Phase 3
7. Policy enforcement parity uneven (core vs plugin controls) -> Phase 3

## Out of Scope

- Multi-agent orchestration.
- Cloud sync and multi-user RBAC.
- Open plugin marketplace features.

## Implementation Transition

Next step: generate a detailed implementation plan using the `writing-plans` skill, using this design as the source of truth.
