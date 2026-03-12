# Gap Remediation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fully close all identified architecture gaps on `master` baseline, with docs-first contract alignment and risk-first sequencing.

**Architecture:** Add a control-plane orchestration path that guarantees context provenance + pre-egress redaction before model calls, then harden patch safety and replay provenance, then align policy and API contracts. Keep each change test-first and small, with explicit invariants enforced by integration tests.

**Tech Stack:** Go, net/http, SQLite (audit store), existing internal packages (`controlplane`, `context`, `patch`, `tools`, `audit`, `tui`).

---

### Task 1: Enforce Session Input Validity and Metadata Integrity

**Files:**
- Modify: `internal/session/manager.go`
- Modify: `internal/session/manager_test.go`
- Modify: `internal/controlplane/schema_validation_test.go`

**Step 1: Write failing tests**
- Add tests for invalid trust tier and empty repo path on session creation.
- Add test that `Get` returns `repo_path` from persisted session start payload.

**Step 2: Run failing tests**
- Run: `go test ./internal/session ./internal/controlplane -run "Session|SessionsEndpoint"`
- Expected: FAIL on new validation/metadata assertions.

**Step 3: Implement minimal validation and metadata reconstruction**
- Enforce trust tier allowlist (`tier0..tier3`) and non-empty repo path in manager create path.
- Ensure session `Get` repopulates `RepoPath` from `session_started` payload.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/session ./internal/controlplane -run "Session|SessionsEndpoint"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/session/manager.go internal/session/manager_test.go internal/controlplane/schema_validation_test.go`
- `git commit -m "fix(session): enforce trust-tier validation and restore repo metadata"`

### Task 2: Wire Turn Context Manifest Into Live Turn Execution

**Files:**
- Create: `internal/controlplane/turn_orchestrator.go`
- Modify: `internal/controlplane/server.go`
- Modify: `internal/controlplane/turns_api_test.go`
- Modify: `internal/context/context_test.go`

**Step 1: Write failing tests**
- Add integration test: creating a turn produces non-empty context manifest entries retrievable via `/sessions/{id}/turns/{id}/context`.
- Add test that model invocation path depends on assembled context envelope, not raw message.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane ./internal/context -run "Context|Turn"`
- Expected: FAIL because manifest is not recorded in live turn path.

**Step 3: Implement orchestrator wiring**
- Add control-plane turn orchestration helper that:
  - resolves session/turn,
  - assembles fragments,
  - records manifest entries,
  - passes assembled prompt content to runner input.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/controlplane ./internal/context -run "Context|Turn"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/turn_orchestrator.go internal/controlplane/server.go internal/controlplane/turns_api_test.go internal/context/context_test.go`
- `git commit -m "feat(context): wire manifest-backed turn orchestration"`

### Task 3: Guarantee Secret Redaction Before Model Egress

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: `internal/controlplane/context_redaction_policy.go`
- Modify: `internal/controlplane/turns_api_test.go`
- Modify: `internal/context/context_test.go`

**Step 1: Write failing tests**
- Add test that secret-like user input is redacted/masked in model-facing prompt content.
- Add test that raw secret never appears in emitted model invocation payload path.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane ./internal/context -run "Redaction|Model"`
- Expected: FAIL with raw-message egress evidence.

**Step 3: Implement minimal egress guard**
- Route all model-bound content through destination-aware redaction policy.
- Remove any direct `req.Message` pass-through into runtime execution.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/controlplane ./internal/context -run "Redaction|Model"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/server.go internal/controlplane/context_redaction_policy.go internal/controlplane/turns_api_test.go internal/context/context_test.go`
- `git commit -m "fix(secrets): enforce pre-egress redaction on model path"`

### Task 4: Add Patch Safety Pipeline (Validator + Checkpoint + Guardrails)

**Files:**
- Create: `internal/tools/patch_safety.go`
- Modify: `internal/tools/patch_handler.go`
- Modify: `internal/tools/patch_handler_test.go`
- Modify: `internal/patch/validator_test.go`
- Modify: `internal/controlplane/server.go`
- Modify: `internal/git/anchors.go`

**Step 1: Write failing tests**
- Add tests that patch apply rejects missing/invalid preimage for update operations.
- Add tests that checkpoint is created before write-class mutation and rollback called on apply failure.
- Add tests that syntax/format checks are invoked when configured.

**Step 2: Run failing tests**
- Run: `go test ./internal/tools ./internal/patch ./internal/git -run "Patch|Checkpoint|Rollback"`
- Expected: FAIL due current direct engine apply path.

**Step 3: Implement minimal safety wrapper**
- Introduce `PatchSafetyService` to orchestrate:
  - proposal extraction,
  - validator pre-checks,
  - checkpoint creation,
  - apply execution,
  - rollback on failure,
  - post-apply verification hooks.
- Use service from `ApplyPatchHandler`.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/tools ./internal/patch ./internal/git -run "Patch|Checkpoint|Rollback"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/tools/patch_safety.go internal/tools/patch_handler.go internal/tools/patch_handler_test.go internal/patch/validator_test.go internal/controlplane/server.go internal/git/anchors.go`
- `git commit -m "feat(patch): enforce preapply validation and checkpoint rollback"`

### Task 5: Complete File Mutation Provenance for Replay Verification

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: `internal/audit/event_store.go`
- Modify: `internal/controlplane/replay_test.go`
- Modify: `pkg/tui/app_test.go`

**Step 1: Write failing tests**
- Add test that file mutation records persist `session_id`, `turn_id`, `before_hash`, `after_hash`, and proposal/patch linkage.
- Add replay test that event payload includes verification fields.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane ./internal/audit ./pkg/tui -run "FileMutation|Replay"`
- Expected: FAIL due incomplete payload linkage.

**Step 3: Implement minimal provenance enrichment**
- Populate mutation recorder callback with session/turn context and hashes.
- Persist enriched audit record and include same verification fields in event payload.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/controlplane ./internal/audit ./pkg/tui -run "FileMutation|Replay"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/server.go internal/audit/event_store.go internal/controlplane/replay_test.go pkg/tui/app_test.go`
- `git commit -m "feat(replay): add verifiable file mutation provenance"`

### Task 6: Enrich Tool Invocation Audit for Reproducibility

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: `internal/audit/event_store.go`
- Modify: `internal/controlplane/tool_execute_compat_test.go`
- Modify: `internal/controlplane/approval_test.go`

**Step 1: Write failing tests**
- Add tests requiring `turn_id`, `approval_id`, input hash/output reference metadata in invocation records/events.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane ./internal/audit -run "ToolInvocation|Approval"`
- Expected: FAIL on missing metadata.

**Step 3: Implement minimal recorder enrichment**
- Extend recorder path to include session/turn/approval causality and normalized references.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/controlplane ./internal/audit -run "ToolInvocation|Approval"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/server.go internal/audit/event_store.go internal/controlplane/tool_execute_compat_test.go internal/controlplane/approval_test.go`
- `git commit -m "feat(audit): enrich tool invocation causality metadata"`

### Task 7: Enforce Policy Parity for Core and Plugin Tools

**Files:**
- Modify: `internal/controlplane/server.go`
- Modify: `internal/controlplane/plugin_approval_scope_test.go`
- Modify: `internal/controlplane/plugin_exposure_test.go`
- Modify: `internal/controlplane/mcp_transport_test.go`

**Step 1: Write failing tests**
- Add tests asserting path/network governance parity between core and plugin tools where side effects are equivalent.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane -run "Plugin|Policy|MCP"`
- Expected: FAIL on current uneven enforcement.

**Step 3: Implement minimal policy parity rules**
- Move shared path/network checks into unified validation helpers used by both core/plugin tool execution paths.

**Step 4: Re-run targeted tests**
- Run: `go test ./internal/controlplane -run "Plugin|Policy|MCP"`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/server.go internal/controlplane/plugin_approval_scope_test.go internal/controlplane/plugin_exposure_test.go internal/controlplane/mcp_transport_test.go`
- `git commit -m "fix(policy): align core and plugin enforcement semantics"`

### Task 8: Final Contract Alignment and End-to-End Gates

**Files:**
- Modify: `internal/controlplane/schema_validation.go`
- Modify: `internal/validation/registry.go`
- Modify: `pkg/tui/client.go`
- Modify: `pkg/tui/client_test.go`
- Modify: `docs/ai_cli_v_1_architecture_checklist.md`
- Modify: `docs/ai_cli_v_1_implementation_plan.md`

**Step 1: Write failing contract tests**
- Add/extend tests for docs-first response shapes across all public endpoints touched by this plan.

**Step 2: Run failing tests**
- Run: `go test ./internal/controlplane ./internal/validation ./pkg/tui -run "Schema|Contract|APIClient"`
- Expected: FAIL before final alignment.

**Step 3: Implement minimal contract fixes**
- Align schema registry and runtime responses.
- Update TUI decoding to exact server envelopes.
- Update docs checklists to mark implemented controls with references.

**Step 4: Full verification**
- Run: `go test ./...`
- Expected: PASS.

**Step 5: Commit**
- `git add internal/controlplane/schema_validation.go internal/validation/registry.go pkg/tui/client.go pkg/tui/client_test.go docs/ai_cli_v_1_architecture_checklist.md docs/ai_cli_v_1_implementation_plan.md`
- `git commit -m "chore(contract): finalize docs-first API and validation alignment"`

### Task 9: Gap Closure Evidence and Release Readiness

**Files:**
- Create: `docs/plans/2026-03-12-gap-remediation-verification.md`
- Modify: `bead-evidence/v1-go-no-go.json`

**Step 1: Write verification matrix**
- Map each identified gap to:
  - implementation commit,
  - test name(s),
  - pass evidence.

**Step 2: Run release verification commands**
- Run: `go test ./...`
- Run: `go test ./internal/controlplane ./internal/context ./internal/tools ./internal/patch ./internal/audit ./pkg/tui`
- Expected: PASS.

**Step 3: Commit evidence updates**
- `git add docs/plans/2026-03-12-gap-remediation-verification.md bead-evidence/v1-go-no-go.json`
- `git commit -m "docs: add architecture gap closure verification evidence"`

## Notes for Implementation

- Keep each task isolated and do not bundle across phases.
- Prioritize test names that encode the specific gap/invariant being enforced.
- Reject speculative features not required for listed gap closure (YAGNI).
- Preserve docs-first exactness even when it requires client updates in this branch.
