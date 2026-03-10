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
