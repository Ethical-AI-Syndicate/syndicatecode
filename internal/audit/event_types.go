package audit

const (
	EventSessionStarted    = "session_started"
	EventSessionTerminated = "session_terminated"

	EventTurnStarted          = "turn_started"
	EventTurnAwaitingApproval = "turn_awaiting_approval"
	EventTurnCompleted        = "turn_completed"
	EventTurnFailed           = "turn_failed"
	EventTurnCancelled        = "turn_cancelled"

	EventApprovalProposed   = "approval_proposed"
	EventApprovalDecided    = "approval_decided"
	EventApprovalExecuted   = "approval_executed"
	EventApprovalTransition = "approval.transition"

	EventMCPCall        = "mcp.call"
	EventToolRedaction  = "tool_output_redaction"
	EventRetentionClean = "retention.cleanup"

	EventContextFragment         = "context_fragment"
	EventContextManifestEntry    = "context_manifest_entry"
	EventContextManifestConflict = "context_manifest_conflict"

	EventModelInvoked = "model_invocation"
	EventToolInvoked  = "tool_invocation"
	EventToolResult   = "tool_result"
	EventFileMutation = "file_mutation"
)
