package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/requestmeta"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/state"
)

type TurnStatus = state.TurnState

type RedactionDestination string

const (
	DestinationPersistence   RedactionDestination = "persistence"
	DestinationModelProvider RedactionDestination = "model_provider"
)

type RedactionDecision struct {
	Content             string
	Action              string
	Denied              bool
	Reason              string
	SensitivityClass    string
	ClassificationLevel string
}

type RedactionPolicy interface {
	Apply(sourceRef, sourceType, content string, destination RedactionDestination) RedactionDecision
}

type defaultDenyRedactionPolicy struct{}

func (defaultDenyRedactionPolicy) Apply(sourceRef, sourceType, content string, destination RedactionDestination) RedactionDecision {
	_ = sourceRef
	_ = sourceType
	_ = content
	_ = destination
	return RedactionDecision{
		Content:             "[DENIED]",
		Action:              "deny",
		Denied:              true,
		Reason:              "redaction policy not configured",
		SensitivityClass:    "A",
		ClassificationLevel: "secret_denied",
	}
}

const (
	TurnStatusActive           TurnStatus = state.TurnStateActive
	TurnStatusAwaitingApproval TurnStatus = state.TurnStateAwaitingApproval
	TurnStatusCompleted        TurnStatus = state.TurnStateCompleted
	TurnStatusFailed           TurnStatus = state.TurnStateFailed
	TurnStatusCancelled        TurnStatus = state.TurnStateCancelled
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
	policy     RedactionPolicy
}

func NewTurnManager(eventStore *audit.EventStore, sessionMgr *session.Manager) *TurnManager {
	return NewTurnManagerWithPolicy(eventStore, sessionMgr, defaultDenyRedactionPolicy{})
}

func NewTurnManagerWithPolicy(eventStore *audit.EventStore, sessionMgr *session.Manager, policy RedactionPolicy) *TurnManager {
	if policy == nil {
		policy = defaultDenyRedactionPolicy{}
	}
	return &TurnManager{
		eventStore: eventStore,
		sessionMgr: sessionMgr,
		policy:     policy,
	}
}

func (m *TurnManager) Create(ctx context.Context, sessionID, message string) (*Turn, error) {
	// Verify session exists
	_, err := m.sessionMgr.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	hasActiveTurn, activeTurnID, err := m.hasActiveMutableTurn(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if hasActiveTurn {
		return nil, fmt.Errorf("%w: session %s already has active turn %s", ErrActiveMutableTurnConflict, sessionID, activeTurnID)
	}

	decision := m.policy.Apply("turn.message", "user_input", message, DestinationPersistence)
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
		"sensitivity_class":    decision.SensitivityClass,
		"classification_level": decision.ClassificationLevel,
		"entity_type":          "turn",
		"entity_id":            turn.ID,
		"previous_state":       "none",
		"next_state":           turn.Status,
		"cause":                "turn_create_requested",
		"transition_timestamp": now.Format(time.RFC3339Nano),
		"related_ids": map[string]interface{}{
			"session_id": sessionID,
		},
	})
	if err != nil {
		return nil, err
	}
	event := audit.Event{
		ID:            uuid.New().String(),
		SessionID:     sessionID,
		TurnID:        turn.ID,
		EventType:     audit.EventTurnStarted,
		Actor:         requestmeta.Actor(ctx),
		Timestamp:     now,
		PolicyVersion: "1.0.0",
		Payload:       payload,
	}

	if err := m.eventStore.Append(ctx, event); err != nil {
		return nil, err
	}

	return turn, nil
}

func (m *TurnManager) hasActiveMutableTurn(ctx context.Context, sessionID string) (bool, string, error) {
	events, err := m.eventStore.QueryBySession(ctx, sessionID)
	if err != nil {
		return false, "", err
	}

	turnStateByID := make(map[string]TurnStatus)
	turnOrder := make([]string, 0)
	seen := make(map[string]struct{})

	for _, event := range events {
		if event.TurnID == "" {
			continue
		}
		if _, ok := seen[event.TurnID]; !ok {
			turnOrder = append(turnOrder, event.TurnID)
			seen[event.TurnID] = struct{}{}
		}

		switch event.EventType {
		case audit.EventTurnStarted:
			if !isTerminalTurnStatus(turnStateByID[event.TurnID]) {
				turnStateByID[event.TurnID] = TurnStatusActive
			}
		case audit.EventTurnAwaitingApproval:
			if !isTerminalTurnStatus(turnStateByID[event.TurnID]) {
				turnStateByID[event.TurnID] = TurnStatusAwaitingApproval
			}
		case audit.EventTurnCompleted:
			turnStateByID[event.TurnID] = TurnStatusCompleted
		case audit.EventTurnFailed:
			turnStateByID[event.TurnID] = TurnStatusFailed
		case audit.EventTurnCancelled:
			turnStateByID[event.TurnID] = TurnStatusCancelled
		default:
			status, ok, decodeErr := transitionStatusFromPayload(event.Payload)
			if decodeErr != nil {
				return false, "", decodeErr
			}
			if ok {
				if isTerminalTurnStatus(turnStateByID[event.TurnID]) {
					continue
				}
				turnStateByID[event.TurnID] = status
			}
		}
	}

	for _, turnID := range turnOrder {
		status := turnStateByID[turnID]
		if status == TurnStatusActive || status == TurnStatusAwaitingApproval {
			return true, turnID, nil
		}
	}

	return false, "", nil
}

func transitionStatusFromPayload(payload []byte) (TurnStatus, bool, error) {
	if len(payload) == 0 {
		return "", false, nil
	}

	var body map[string]interface{}
	if err := json.Unmarshal(payload, &body); err != nil {
		return "", false, fmt.Errorf("failed to decode turn transition payload: %w", err)
	}

	rawStatus, ok := body["next_state"]
	if !ok {
		return "", false, nil
	}
	status, ok := rawStatus.(string)
	if !ok {
		return "", false, nil
	}

	return TurnStatus(status), true, nil
}

func isTerminalTurnStatus(status TurnStatus) bool {
	return status == TurnStatusCompleted || status == TurnStatusFailed || status == TurnStatusCancelled
}

func (m *TurnManager) Get(ctx context.Context, turnID string) (*Turn, error) {
	events, err := m.eventStore.QueryByTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}

	statusByID := deriveTurnStatuses(events)

	for _, e := range events {
		if e.EventType == audit.EventTurnStarted {
			var payload struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return nil, err
			}

			updatedAt := e.Timestamp
			for _, later := range events {
				if later.Timestamp.After(updatedAt) {
					updatedAt = later.Timestamp
				}
			}

			status := statusByID[turnID]
			if status == "" {
				status = TurnStatusActive
			}

			return &Turn{
				ID:        turnID,
				SessionID: e.SessionID,
				Message:   payload.Message,
				Status:    status,
				CreatedAt: e.Timestamp,
				UpdatedAt: updatedAt,
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

	statusByID := deriveTurnStatuses(events)

	turns := make(map[string]*Turn)
	for _, e := range events {
		if e.EventType == audit.EventTurnStarted {
			var payload struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return nil, err
			}

			status := statusByID[e.TurnID]
			if status == "" {
				status = TurnStatusActive
			}

			turns[e.TurnID] = &Turn{
				ID:        e.TurnID,
				SessionID: sessionID,
				Message:   payload.Message,
				Status:    status,
				CreatedAt: e.Timestamp,
				UpdatedAt: e.Timestamp,
			}
			continue
		}

		if existing, ok := turns[e.TurnID]; ok {
			if e.Timestamp.After(existing.UpdatedAt) {
				existing.UpdatedAt = e.Timestamp
			}
			if status := statusByID[e.TurnID]; status != "" {
				existing.Status = status
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
var ErrActiveMutableTurnConflict = errors.New("active mutable turn already exists")

func deriveTurnStatuses(events []audit.Event) map[string]TurnStatus {
	statuses := make(map[string]TurnStatus)
	for _, event := range events {
		if event.TurnID == "" {
			continue
		}
		switch event.EventType {
		case audit.EventTurnStarted:
			if statuses[event.TurnID] == "" {
				statuses[event.TurnID] = TurnStatusActive
			}
		case audit.EventTurnAwaitingApproval:
			statuses[event.TurnID] = TurnStatusAwaitingApproval
		case audit.EventTurnCompleted:
			statuses[event.TurnID] = TurnStatusCompleted
		case audit.EventTurnFailed:
			statuses[event.TurnID] = TurnStatusFailed
		case audit.EventTurnCancelled:
			statuses[event.TurnID] = TurnStatusCancelled
		}
	}
	return statuses
}

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
	policy    RedactionPolicy
}

// NewContextAssembler creates a new context assembler with the given token budget
func NewContextAssembler(budget int) *ContextAssembler {
	return NewContextAssemblerWithPolicy(budget, defaultDenyRedactionPolicy{})
}

func NewContextAssemblerWithPolicy(budget int, policy RedactionPolicy) *ContextAssembler {
	if policy == nil {
		policy = defaultDenyRedactionPolicy{}
	}
	return &ContextAssembler{
		budget:    budget,
		fragments: make([]*ContextFragment, 0),
		policy:    policy,
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

// AssembleFromRanked uses BudgetAllocator to select fragments within category budgets.
// Use this instead of AddFragment when ranked fragments are available.
func (a *ContextAssembler) AssembleFromRanked(ranked []RankedFragment, budget CategoryBudget) []ContextFragment {
	allocator := NewBudgetAllocator(budget)
	allocated, _ := allocator.AllocateFragments(ranked)
	a.fragments = make([]*ContextFragment, 0, len(allocated))
	for i := range allocated {
		a.fragments = append(a.fragments, &allocated[i])
	}
	return allocated
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
		decision := a.policy.Apply(f.SourceRef, f.SourceType, f.Content, DestinationModelProvider)
		f.RedactionAction = decision.Action
		f.RedactionDenied = decision.Denied
		f.RedactionReason = decision.Reason
		f.Sensitivity = decision.SensitivityClass
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
			EventType:     audit.EventContextFragment,
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
	events, err := m.eventStore.QueryByTurn(ctx, turnID)
	if err != nil {
		return nil, err
	}

	var fragments []ContextFragment
	for _, e := range events {
		if e.EventType == audit.EventContextFragment {
			var f ContextFragment
			if err := json.Unmarshal(e.Payload, &f); err != nil {
				return nil, err
			}
			fragments = append(fragments, f)
		}
	}

	return fragments, nil
}
