package context

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

type TurnStatus string

const (
	TurnStatusActive    TurnStatus = "active"
	TurnStatusCompleted TurnStatus = "completed"
	TurnStatusFailed    TurnStatus = "failed"
)

type Turn struct {
	ID        string     `json:"turn_id"`
	SessionID string     `json:"session_id"`
	Message   string     `json:"message"`
	Status    TurnStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type RedactionDecision struct {
	Content             string
	Action              string
	Denied              bool
	Reason              string
	Sensitivity         string
	ClassificationLevel string
}

type RedactorFunc func(sourceRef, sourceType, content string) RedactionDecision

func defaultPersistenceRedactor(_ string, _ string, content string) RedactionDecision {
	hash := sha256.Sum256([]byte(content))
	return RedactionDecision{
		Content:     fmt.Sprintf("sha256:%x", hash),
		Action:      "hash",
		Sensitivity: "unknown",
		Reason:      "default persistence safeguard",
	}
}

func defaultModelProviderRedactor(_ string, _ string, _ string) RedactionDecision {
	return RedactionDecision{
		Content:     "",
		Action:      "deny",
		Denied:      true,
		Reason:      "default model provider safeguard",
		Sensitivity: "unknown",
	}
}

type TurnManager struct {
	eventStore *audit.EventStore
	sessionMgr *session.Manager
	redactor   RedactorFunc
}

func NewTurnManager(eventStore *audit.EventStore, sessionMgr *session.Manager) *TurnManager {
	return NewTurnManagerWithRedactor(eventStore, sessionMgr, defaultPersistenceRedactor)
}

func NewTurnManagerWithRedactor(eventStore *audit.EventStore, sessionMgr *session.Manager, redactor RedactorFunc) *TurnManager {
	if redactor == nil {
		redactor = defaultPersistenceRedactor
	}

	return &TurnManager{
		eventStore: eventStore,
		sessionMgr: sessionMgr,
		redactor:   redactor,
	}
}

func (m *TurnManager) Create(ctx context.Context, sessionID, message string) (*Turn, error) {
	// Verify session exists
	_, err := m.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	decision := m.redactor("turn.message", "user_input", message)
	redactedMessage := decision.Content
	now := time.Now()
	turn := &Turn{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Message:   redactedMessage,
		Status:    TurnStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	payload, err := json.Marshal(map[string]interface{}{
		"message":              redactedMessage,
		"redaction_action":     decision.Action,
		"redaction_denied":     decision.Denied,
		"redaction_reason":     decision.Reason,
		"sensitivity_class":    decision.Sensitivity,
		"classification_level": decision.ClassificationLevel,
	})
	if err != nil {
		return nil, err
	}
	event := audit.Event{
		ID:            uuid.New().String(),
		SessionID:     sessionID,
		TurnID:        turn.ID,
		EventType:     "turn_started",
		Actor:         "user",
		Timestamp:     now,
		PolicyVersion: "1.0.0",
		Payload:       payload,
	}

	if err := m.eventStore.Append(ctx, event); err != nil {
		return nil, err
	}

	return turn, nil
}

func (m *TurnManager) Get(ctx context.Context, turnID string) (*Turn, error) {
	events, err := m.eventStore.QueryAll(ctx)
	if err != nil {
		return nil, err
	}

	for _, e := range events {
		if e.TurnID == turnID && e.EventType == "turn_started" {
			var payload struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return nil, err
			}

			return &Turn{
				ID:        turnID,
				SessionID: e.SessionID,
				Message:   payload.Message,
				Status:    TurnStatusActive,
				CreatedAt: e.Timestamp,
				UpdatedAt: e.Timestamp,
			}, nil
		}
	}

	return nil, ErrTurnNotFound
}

func (m *TurnManager) ListBySession(ctx context.Context, sessionID string) ([]*Turn, error) {
	events, err := m.eventStore.QueryBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	turns := make(map[string]*Turn)
	for _, e := range events {
		if e.EventType == "turn_started" {
			var payload struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return nil, err
			}

			turns[e.TurnID] = &Turn{
				ID:        e.TurnID,
				SessionID: sessionID,
				Message:   payload.Message,
				Status:    TurnStatusActive,
				CreatedAt: e.Timestamp,
				UpdatedAt: e.Timestamp,
			}
		}
	}

	result := make([]*Turn, 0, len(turns))
	for _, t := range turns {
		result = append(result, t)
	}

	return result, nil
}

var ErrTurnNotFound = errors.New("turn not found")

// ContextFragment represents a piece of context included in the prompt
type ContextFragment struct {
	SourceType      string            `json:"source_type"` // file, tool_output, instruction, git
	SourceRef       string            `json:"source_ref"`
	Content         string            `json:"content"`
	TokenCount      int               `json:"token_count"`
	Included        bool              `json:"included"`
	ExclusionReason string            `json:"exclusion_reason,omitempty"`
	Truncated       bool              `json:"truncated"`
	InclusionReason string            `json:"inclusion_reason"` // user_requested, auto, priority
	Sensitivity     string            `json:"sensitivity"`
	FreshnessState  string            `json:"freshness_state"`
	Conflicts       []ContextConflict `json:"conflicts,omitempty"`
	RedactionAction string            `json:"redaction_action,omitempty"`
	RedactionDenied bool              `json:"redaction_denied"`
	RedactionReason string            `json:"redaction_reason,omitempty"`
}

type ContextConflict struct {
	WithSourceRef string `json:"with_source_ref"`
	Reason        string `json:"reason"`
}

// ContextAssembler assembles context from fragments with token budgeting
type ContextAssembler struct {
	budget    int
	fragments []*ContextFragment
	redactor  RedactorFunc
}

// NewContextAssembler creates a new context assembler with the given token budget
func NewContextAssembler(budget int) *ContextAssembler {
	return NewContextAssemblerWithRedactor(budget, defaultModelProviderRedactor)
}

func NewContextAssemblerWithRedactor(budget int, redactor RedactorFunc) *ContextAssembler {
	if redactor == nil {
		redactor = defaultModelProviderRedactor
	}

	return &ContextAssembler{
		budget:    budget,
		fragments: make([]*ContextFragment, 0),
		redactor:  redactor,
	}
}

// AddFragment adds a context fragment to the assembler
func (a *ContextAssembler) AddFragment(fragment *ContextFragment) error {
	// Check if adding this fragment would exceed budget
	currentTokens := 0
	for _, f := range a.fragments {
		currentTokens += f.TokenCount
	}

	if currentTokens+fragment.TokenCount > a.budget {
		// Mark as truncated and still add it
		fragment.Truncated = true
	}

	a.fragments = append(a.fragments, fragment)
	return nil
}

// Fragments returns all fragments
func (a *ContextAssembler) Fragments() []*ContextFragment {
	return a.fragments
}

// BuildPrompt builds the final prompt from fragments
func (a *ContextAssembler) BuildPrompt() string {
	// Sort by priority: instruction > file > git > tool_output
	sort.Slice(a.fragments, func(i, j int) bool {
		priority := map[string]int{
			"instruction": 4,
			"file":        3,
			"git":         2,
			"tool_output": 1,
		}
		return priority[a.fragments[i].SourceType] > priority[a.fragments[j].SourceType]
	})

	var parts []string
	for _, f := range a.fragments {
		decision := a.redactor(f.SourceRef, f.SourceType, f.Content)
		f.RedactionAction = decision.Action
		f.RedactionDenied = decision.Denied
		f.RedactionReason = decision.Reason
		f.Sensitivity = decision.Sensitivity
		if decision.Denied {
			f.Included = false
			f.ExclusionReason = "policy_denied"
			continue
		}
		f.Included = true
		f.Content = decision.Content
		parts = append(parts, decision.Content)
	}

	return strings.Join(parts, "\n\n")
}

// TokenBudget manages token allocation
type TokenBudget struct {
	total       int
	used        int
	allocations map[string]int
}

func NewTokenBudget(total int) *TokenBudget {
	return &TokenBudget{
		total:       total,
		used:        0,
		allocations: make(map[string]int),
	}
}

func (b *TokenBudget) Allocate(category string, tokens int) error {
	if b.used+tokens > b.total {
		return errors.New("token budget exceeded")
	}
	b.used += tokens
	b.allocations[category] = tokens
	return nil
}

func (b *TokenBudget) Used() int {
	return b.used
}

func (b *TokenBudget) Remaining() int {
	return b.total - b.used
}

func (b *TokenBudget) Total() int {
	return b.total
}

// ContextManifest records context fragments for a turn
type ContextManifest struct {
	eventStore *audit.EventStore
}

func NewContextManifest(eventStore *audit.EventStore) *ContextManifest {
	return &ContextManifest{eventStore: eventStore}
}

func (m *ContextManifest) Record(ctx context.Context, sessionID, turnID string, fragments []ContextFragment) error {
	now := time.Now()
	for _, f := range fragments {
		payload, err := json.Marshal(f)
		if err != nil {
			return err
		}
		event := audit.Event{
			ID:            uuid.New().String(),
			SessionID:     sessionID,
			TurnID:        turnID,
			EventType:     "context_fragment",
			Actor:         "system",
			Timestamp:     now,
			PolicyVersion: "1.0.0",
			Payload:       payload,
		}
		if err := m.eventStore.Append(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (m *ContextManifest) Get(ctx context.Context, turnID string) ([]ContextFragment, error) {
	events, err := m.eventStore.QueryAll(ctx)
	if err != nil {
		return nil, err
	}

	var fragments []ContextFragment
	for _, e := range events {
		if e.TurnID == turnID && e.EventType == "context_fragment" {
			var f ContextFragment
			if err := json.Unmarshal(e.Payload, &f); err != nil {
				return nil, err
			}
			fragments = append(fragments, f)
		}
	}

	return fragments, nil
}
