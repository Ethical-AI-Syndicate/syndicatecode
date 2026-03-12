# Gap Remediation Verification

Date: 2026-03-12
Baseline: `master`
Plan: `docs/plans/2026-03-12-gap-remediation-implementation-plan.md`

## Verification Matrix

1. Context inspectability wired end-to-end
- Implementation: `internal/controlplane/turn_orchestrator.go`, `internal/controlplane/server.go`
- Tests: `TestTurnCreation_RecordsManifestAndUsesAssembledPrompt` (`internal/controlplane/turns_api_test.go`)

2. Secrets filtered before model egress
- Implementation: `internal/controlplane/turn_orchestrator.go`
- Tests: `TestTurnCreation_RedactsSecretBeforeModelEgress` (`internal/controlplane/turns_api_test.go`)

3. Safe editing guardrails (preimage/checkpoint/rollback path)
- Implementation: `internal/tools/patch_safety.go`, `internal/tools/patch_handler.go`, `internal/git/anchors.go`
- Tests:
  - `TestApplyPatchHandler_RestoresCheckpointOnApplyFailure` (`internal/tools/patch_handler_test.go`)
  - `TestCreateAnchorFailsWhenGitRefEnabledWithoutCheckpointStore` (`internal/git/anchors_test.go`)

4. File mutation provenance completeness
- Implementation: `internal/controlplane/server.go`, `internal/tools/patch_handler.go`
- Tests:
  - `TestApplyPatchHandler_RecordsBeforeAndAfterHashes` (`internal/tools/patch_handler_test.go`)
  - `TestHandleSessionByID_EventsIncludeFileMutationHashes` (`internal/controlplane/replay_test.go`)

5. Tool invocation reproducibility metadata
- Implementation: `internal/controlplane/server.go`
- Tests: `TestAuditExecutionRecorder_EmitsTurnAndApprovalMetadata` (`internal/controlplane/tool_invocation_audit_test.go`)

6. Session input and metadata integrity
- Implementation: `internal/session/manager.go`, `internal/controlplane/server.go`
- Tests:
  - `TestManager_CreateRejectsInvalidTrustTier` (`internal/session/manager_test.go`)
  - `TestManager_CreateRejectsEmptyRepoPath` (`internal/session/manager_test.go`)
  - `TestManager_GetRestoresRepoPathFromPayload` (`internal/session/manager_test.go`)
  - `TestSessionsEndpoint_ReturnsBadRequestForInvalidTrustTier` (`internal/controlplane/schema_validation_test.go`)

7. Policy parity (core vs plugin scope enforcement)
- Implementation: `internal/controlplane/server.go`
- Tests: `TestHandleToolExecute_CoreAndPluginMetadataScopeParity` (`internal/controlplane/policy_parity_test.go`)

## Contract and Client Alignment Evidence

- Event type discoverability endpoint and validation:
  - `internal/controlplane/server.go`
  - `internal/controlplane/event_types_endpoint_test.go`
- TUI wrapper-response decoding parity:
  - `pkg/tui/client.go`
  - `pkg/tui/client_test.go`
  - `pkg/tui/app_test.go`
- Validation registry and schema updates:
  - `internal/validation/registry.go`
  - `internal/controlplane/schema_validation.go`

## Test Command Evidence

- `go test ./internal/controlplane -run "Plugin|Policy|MCP|Parity"`
- `go test ./internal/controlplane ./internal/validation ./pkg/tui -run "Schema|Contract|APIClient|EventTypes|SessionsEndpoint"`
- `go test ./...`

All commands pass in the current branch state.
