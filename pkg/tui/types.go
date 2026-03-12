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
	ExpiresAt  string `json:"expires_at,omitempty"`
	ArgsHash   string `json:"arguments_hash,omitempty"`
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

type ToolDefinition struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	SideEffect       string `json:"side_effect"`
	ApprovalRequired bool   `json:"approval_required"`
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

type LSPRange struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

type LSPDiagnostic struct {
	Path     string   `json:"path"`
	Code     string   `json:"code,omitempty"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Range    LSPRange `json:"range"`
}

type LSPSymbol struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Path  string   `json:"path"`
	Range LSPRange `json:"range"`
}

type LSPPositionRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
}

type LSPHoverResponse struct {
	Contents string   `json:"contents"`
	Range    LSPRange `json:"range"`
}

type LSPLocation struct {
	Path  string   `json:"path"`
	Range LSPRange `json:"range"`
}

type LSPCompletionItem struct {
	Label         string `json:"label"`
	Kind          string `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}
