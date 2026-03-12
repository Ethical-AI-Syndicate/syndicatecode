package agent

import (
	"context"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/trust"
)

type Config struct {
	MaxDepth       int
	MaxToolCalls   int
	MaxRetries     int
	TurnTimeout    time.Duration
	MaxOutputBytes int
}

func DefaultConfig(trustTier string) Config {
	p := trust.DefaultPolicy()
	maxOutput := 1024 * 1024
	turnTimeout := 600 * time.Second
	maxRetries := 3
	switch trustTier {
	case "tier0":
		maxOutput = 64 * 1024
		turnTimeout = 30 * time.Second
		maxRetries = 1
	case "tier1":
		maxOutput = 256 * 1024
		turnTimeout = 120 * time.Second
		maxRetries = 2
	case "tier2":
		maxOutput = 512 * 1024
		turnTimeout = 300 * time.Second
	}
	return Config{MaxDepth: p.MaxLoopDepth(trustTier), MaxToolCalls: p.MaxToolCalls(trustTier), MaxRetries: maxRetries, TurnTimeout: turnTimeout, MaxOutputBytes: maxOutput}
}

type AgentTurn struct {
	ID        string
	SessionID string
	Message   string
	TrustTier string
	Files     []string
}

type AgentEventType string

const (
	EventTextDelta        AgentEventType = "text_delta"
	EventToolUseStart     AgentEventType = "tool_use_start"
	EventToolInputDelta   AgentEventType = "tool_input_delta"
	EventTurnCompleted    AgentEventType = "turn_completed"
	EventTurnFailed       AgentEventType = "turn_failed"
	EventAwaitingApproval AgentEventType = "awaiting_approval"
)

type AgentEvent struct {
	Type       AgentEventType
	Data       string
	StopReason string
	ToolID     string
	ToolName   string
	ApprovalID string
}

type ApprovalGate interface {
	RequestApproval(ctx context.Context, call tools.ToolCall) (ApprovalResult, error)
}

type ApprovalResult struct {
	Approved   bool
	Reason     string
	ApprovalID string
}

type EventEmitter interface {
	Emit(sessionID string, event AgentEvent)
}
