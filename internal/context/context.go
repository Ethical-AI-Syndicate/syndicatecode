package context

import (
	"context"
	"encoding/json"
	"errors"

	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/syndicatecode/syndicatecode/internal/audit"
	"github.com/syndicatecode/syndicatecode/internal/session"
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

type TurnManager struct {
	eventStore *audit.EventStore
	sessionMgr *session.Manager
}

func NewTurnManager(eventStore *audit.EventStore, sessionMgr *session.Manager) *TurnManager {
	return &TurnManager{
		eventStore: eventStore,
		sessionMgr: sessionMgr,
	}
}

func (m *TurnManager) Create(ctx context.Context, sessionID, message string) (*Turn, error) {
	// Verify session exists
	_, err := m.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	turn := &Turn{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Message:   message,
		Status:    TurnStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}

	payload, _ := json.Marshal(map[string]string{"message": message})
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
	SourceType      string `json:"source_type"` // file, tool_output, instruction, git
	SourceRef       string `json:"source_ref"`
	Content         string `json:"content"`
	TokenCount      int    `json:"token_count"`
	Truncated       bool   `json:"truncated"`
	InclusionReason string `json:"inclusion_reason"` // user_requested, auto, priority
}

// ContextAssembler assembles context from fragments with token budgeting
type ContextAssembler struct {
	budget    int
	fragments []*ContextFragment
}

// NewContextAssembler creates a new context assembler with the given token budget
func NewContextAssembler(budget int) *ContextAssembler {
	return &ContextAssembler{
		budget:    budget,
		fragments: make([]*ContextFragment, 0),
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
		parts = append(parts, f.Content)
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
		payload, _ := json.Marshal(f)
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
