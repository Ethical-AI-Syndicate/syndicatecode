package tui

import "encoding/json"

type Session struct {
	ID        string `json:"id"`
	RepoPath  string `json:"repo_path"`
	TrustTier string `json:"trust_tier"`
	Status    string `json:"status"`
}

type Turn struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	Status    string `json:"status"`
}

type Approval struct {
	ID         string `json:"approval_id"`
	ToolName   string `json:"tool_name"`
	State      string `json:"state"`
	SideEffect string `json:"side_effect"`
}

type ContextFragment struct {
	SourceType string `json:"source_type"`
	SourceRef  string `json:"source_ref"`
	TokenCount int    `json:"token_count"`
	Truncated  bool   `json:"truncated"`
}

type ReplayEvent struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Timestamp string          `json:"timestamp"`
	EventType string          `json:"event_type"`
	Actor     string          `json:"actor"`
	Payload   json.RawMessage `json:"payload"`
}

type PolicyDocument map[string]interface{}

type CreateSessionRequest struct {
	RepoPath  string `json:"repo_path"`
	TrustTier string `json:"trust_tier"`
}

type CreateTurnRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

type DecideApprovalRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}
