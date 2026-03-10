package controlplane

import "testing"

// TestContractAndLifecycleConformance_Bead_l3d_13_2 provides a single
// integration-style entry point for API contract and lifecycle regressions.
func TestContractAndLifecycleConformance_Bead_l3d_13_2(t *testing.T) {
	t.Run("schema contract rejects missing fields", TestSessionsEndpoint_ReturnsInvalidSchemaForMissingFields)
	t.Run("approval flow transitions pending-approved-executed", TestHandleApprovalsAndDecision)
	t.Run("session replay stream ordering", TestHandleSessionByID_EventsReturnsReplayStream)
	t.Run("event stream resume cursor behavior", TestHandleEventStream_DeliversEventsAndResumes_Bead_l3d_2_4)
}

// TestAdversarialSecurityRegressionSuite_Bead_l3d_13_3 validates representative
// policy-bypass attempts stay fail-closed across redaction, plugin trust, and MCP routing.
func TestAdversarialSecurityRegressionSuite_Bead_l3d_13_3(t *testing.T) {
	t.Run("secret egress redacted from tool output", TestHandleToolExecuteRedactsSecrets)
	t.Run("redaction event payload remains secret safe", TestHandleToolExecuteRecordsAuditSafeRedactionEvents)
	t.Run("remote MCP destination denied when not allowlisted", TestHandleToolExecute_RemoteMCPDestinationDeniedWhenNotAllowlisted)
	t.Run("plugin hidden for restricted trust tier", TestHandleToolExecute_PluginInstalledButHiddenForRestrictedSession)
	t.Run("plugin scope rejects implicit full context", TestHandleToolExecute_PluginRejectsImplicitFullContext)
}
