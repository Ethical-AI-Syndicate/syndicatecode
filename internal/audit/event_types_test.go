package audit

import "testing"

func TestEventTypesConstants_Bead_l3d_17_1(t *testing.T) {
	tests := []struct {
		name  string
		event string
	}{
		{"SessionStarted", EventSessionStarted},
		{"SessionTerminated", EventSessionTerminated},
		{"TurnStarted", EventTurnStarted},
		{"TurnAwaitingApproval", EventTurnAwaitingApproval},
		{"TurnCompleted", EventTurnCompleted},
		{"TurnFailed", EventTurnFailed},
		{"TurnCancelled", EventTurnCancelled},
		{"ApprovalProposed", EventApprovalProposed},
		{"ApprovalDecided", EventApprovalDecided},
		{"ApprovalExecuted", EventApprovalExecuted},
		{"ApprovalTransition", EventApprovalTransition},
		{"MCPCall", EventMCPCall},
		{"ToolRedaction", EventToolRedaction},
		{"RetentionClean", EventRetentionClean},
		{"ContextFragment", EventContextFragment},
		{"ContextManifestEntry", EventContextManifestEntry},
		{"ContextManifestConflict", EventContextManifestConflict},
		{"ModelInvoked", EventModelInvoked},
		{"ToolInvoked", EventToolInvoked},
		{"ToolResult", EventToolResult},
		{"FileMutation", EventFileMutation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event == "" {
				t.Errorf("event type %s should not be empty", tt.name)
			}
		})
	}
}

func TestEventTypesNonEmpty_Bead_l3d_17_1(t *testing.T) {
	events := []string{
		EventSessionStarted,
		EventSessionTerminated,
		EventTurnStarted,
		EventTurnAwaitingApproval,
		EventTurnCompleted,
		EventTurnFailed,
		EventTurnCancelled,
		EventApprovalProposed,
		EventApprovalDecided,
		EventApprovalExecuted,
		EventApprovalTransition,
		EventMCPCall,
		EventToolRedaction,
		EventRetentionClean,
		EventContextFragment,
		EventContextManifestEntry,
		EventContextManifestConflict,
		EventModelInvoked,
		EventToolInvoked,
		EventToolResult,
		EventFileMutation,
	}

	for _, event := range events {
		if event == "" {
			t.Error("event type should not be empty")
		}
	}
}
