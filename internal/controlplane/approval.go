package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/state"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type ApprovalState = state.ApprovalState

const (
	ApprovalStateProposed  ApprovalState = state.ApprovalStateProposed
	ApprovalStatePending   ApprovalState = state.ApprovalStatePending
	ApprovalStateApproved  ApprovalState = state.ApprovalStateApproved
	ApprovalStateDenied    ApprovalState = state.ApprovalStateDenied
	ApprovalStateExecuted  ApprovalState = state.ApprovalStateExecuted
	ApprovalStateCancelled ApprovalState = state.ApprovalStateCancelled
)

var (
	ErrApprovalNotFound      = errors.New("approval not found")
	ErrApprovalExpired       = errors.New("approval expired")
	ErrInvalidApprovalState  = errors.New("invalid approval state transition")
	ErrInvalidApprovalAction = errors.New("invalid approval decision")
)

type Approval struct {
	ID             string           `json:"approval_id"`
	SessionID      string           `json:"session_id,omitempty"`
	ToolName       string           `json:"tool_name"`
	ArgumentsHash  string           `json:"arguments_hash"`
	SideEffect     tools.SideEffect `json:"side_effect"`
	AffectedPaths  []string         `json:"paths,omitempty"`
	State          ApprovalState    `json:"state"`
	DecisionReason string           `json:"decision_reason,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	ExpiresAt      time.Time        `json:"expires_at"`
	Call           tools.ToolCall   `json:"call"`
}

type ApprovalManager struct {
	mu     sync.RWMutex
	items  map[string]*Approval
	expiry time.Duration
}

func NewApprovalManager(expiry time.Duration) *ApprovalManager {
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	return &ApprovalManager{
		items:  make(map[string]*Approval),
		expiry: expiry,
	}
}

func (m *ApprovalManager) Propose(sessionID string, call tools.ToolCall, sideEffect tools.SideEffect, paths []string) (*Approval, error) {
	if call.ToolName == "" {
		return nil, errors.New("tool name is required")
	}

	hash, err := hashToolCall(call)
	if err != nil {
		return nil, fmt.Errorf("failed to hash arguments: %w", err)
	}

	now := time.Now().UTC()
	approval := &Approval{
		ID:            fmt.Sprintf("apr-%d", now.UnixNano()),
		SessionID:     sessionID,
		ToolName:      call.ToolName,
		ArgumentsHash: hash,
		SideEffect:    sideEffect,
		AffectedPaths: append([]string(nil), paths...),
		State:         ApprovalStatePending,
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(m.expiry),
		Call:          call,
	}

	m.mu.Lock()
	m.items[approval.ID] = approval
	m.mu.Unlock()

	copy := *approval
	return &copy, nil
}

func (m *ApprovalManager) ListPending(sessionID string) []Approval {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()

	pending := make([]Approval, 0)
	for _, approval := range m.items {
		if approval.State == ApprovalStatePending && now.After(approval.ExpiresAt) {
			approval.State = ApprovalStateCancelled
			approval.UpdatedAt = now
		}
		if approval.State != ApprovalStatePending {
			continue
		}
		if sessionID != "" && approval.SessionID != sessionID {
			continue
		}
		pending = append(pending, *approval)
	}

	return pending
}

func (m *ApprovalManager) Get(id string) (Approval, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	approval, ok := m.items[id]
	if !ok {
		return Approval{}, false
	}
	return *approval, true
}

func (m *ApprovalManager) Decide(id, decision, reason string) (*Approval, error) {
	now := time.Now().UTC()

	m.mu.Lock()
	defer m.mu.Unlock()

	approval, ok := m.items[id]
	if !ok {
		return nil, ErrApprovalNotFound
	}
	if approval.State != ApprovalStatePending {
		return nil, ErrInvalidApprovalState
	}
	if now.After(approval.ExpiresAt) {
		approval.State = ApprovalStateCancelled
		approval.UpdatedAt = now
		return nil, ErrApprovalExpired
	}

	var nextState ApprovalState
	switch decision {
	case "approve":
		nextState = ApprovalStateApproved
	case "deny":
		nextState = ApprovalStateDenied
		approval.DecisionReason = reason
	default:
		return nil, ErrInvalidApprovalAction
	}

	if err := state.ValidateApprovalTransition(approval.State, nextState); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidApprovalState, err)
	}

	approval.State = nextState

	approval.UpdatedAt = now
	copy := *approval
	return &copy, nil
}

func (m *ApprovalManager) MarkExecuted(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	approval, ok := m.items[id]
	if !ok {
		return ErrApprovalNotFound
	}
	if approval.State != ApprovalStateApproved {
		return ErrInvalidApprovalState
	}

	if err := state.ValidateApprovalTransition(approval.State, ApprovalStateExecuted); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidApprovalState, err)
	}

	approval.State = ApprovalStateExecuted
	approval.UpdatedAt = time.Now().UTC()
	return nil
}

func hashToolCall(call tools.ToolCall) (string, error) {
	encoded, err := json.Marshal(call)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
