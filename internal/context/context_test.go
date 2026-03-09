package context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/secrets"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

type fakeRedactionPolicy struct {
	decision RedactionDecision
}

type secretsRedactionPolicy struct {
	executor *secrets.PolicyExecutor
}

func (p secretsRedactionPolicy) Apply(sourceRef, sourceType, content string, destination RedactionDestination) RedactionDecision {
	secretDestination := secrets.DestinationPersistence
	if destination == DestinationModelProvider {
		secretDestination = secrets.DestinationModelProvider
	}

	decision := p.executor.Apply(sourceRef, sourceType, content, secretDestination)
	return RedactionDecision{
		Content:             decision.Content,
		Action:              string(decision.Action),
		Denied:              decision.Denied,
		Reason:              decision.Reason,
		SensitivityClass:    string(decision.Classification.Class),
		ClassificationLevel: string(decision.Classification.Level),
	}
}

func (p fakeRedactionPolicy) Apply(sourceRef, sourceType, content string, destination RedactionDestination) RedactionDecision {
	_ = sourceRef
	_ = sourceType
	_ = content
	_ = destination
	return p.decision
}

func TestTurnManager_Create_UsesInjectedRedactionPolicy(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, fakeRedactionPolicy{decision: RedactionDecision{
		Content:             "[REDACTED]",
		Action:              "hash",
		Denied:              false,
		Reason:              "policy applied",
		SensitivityClass:    "A",
		ClassificationLevel: "secret_denied",
	}})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "my key is AKIA1234567890ABCDEF")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}
	if turn.Message != "[REDACTED]" {
		t.Fatalf("expected injected policy message, got %s", turn.Message)
	}

	events, err := eventStore.QueryBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	var payload map[string]interface{}
	for _, event := range events {
		if event.EventType != "turn_started" {
			continue
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
	}

	if payload["redaction_action"] != "hash" {
		t.Fatalf("expected redaction_action hash, got %v", payload["redaction_action"])
	}
	if payload["sensitivity_class"] != "A" {
		t.Fatalf("expected sensitivity_class A, got %v", payload["sensitivity_class"])
	}
	if payload["classification_level"] != "secret_denied" {
		t.Fatalf("expected classification_level secret_denied, got %v", payload["classification_level"])
	}
}

func TestTurnManager_DefaultPolicyDeniesWhenNotConfigured(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManager(eventStore, sessionMgr)

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "hello")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if turn.Message != "[DENIED]" {
		t.Fatalf("expected secure default to deny content, got %s", turn.Message)
	}
}

func TestTurnManager_Create(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()

	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "Fix the auth bug")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if turn.SessionID != sess.ID {
		t.Errorf("expected session ID %s, got %s", sess.ID, turn.SessionID)
	}

	if turn.Message != "Fix the auth bug" {
		t.Errorf("expected message 'Fix the auth bug', got %s", turn.Message)
	}

	if turn.Status != TurnStatusActive {
		t.Errorf("expected status Active, got %v", turn.Status)
	}
}

func TestTurnManager_CreateRedactsSecrets(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "my key is AKIA1234567890ABCDEF")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if turn.Message == "my key is AKIA1234567890ABCDEF" {
		t.Fatalf("expected message to be redacted")
	}
	if !strings.HasPrefix(turn.Message, "sha256:") {
		t.Fatalf("expected persistence-safe hashed content, got %s", turn.Message)
	}
}

func TestTurnManager_Get(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()

	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	created, err := turnMgr.Create(ctx, sess.ID, "Test message")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	retrieved, err := turnMgr.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("failed to get turn: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected turn ID %s, got %s", created.ID, retrieved.ID)
	}

	if retrieved.Message != "Test message" {
		t.Errorf("expected message 'Test message', got %s", retrieved.Message)
	}
}

func TestTurnManager_ListBySession(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()

	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	firstTurn, err := turnMgr.Create(ctx, sess.ID, "First message")
	if err != nil {
		t.Fatalf("failed to create first turn: %v", err)
	}
	if err := appendTurnCompletedEvent(ctx, eventStore, sess.ID, firstTurn.ID); err != nil {
		t.Fatalf("failed to complete first turn: %v", err)
	}

	secondTurn, err := turnMgr.Create(ctx, sess.ID, "Second message")
	if err != nil {
		t.Fatalf("failed to create second turn: %v", err)
	}
	if err := appendTurnCompletedEvent(ctx, eventStore, sess.ID, secondTurn.ID); err != nil {
		t.Fatalf("failed to complete second turn: %v", err)
	}

	_, err = turnMgr.Create(ctx, sess.ID, "Third message")
	if err != nil {
		t.Fatalf("failed to create third turn: %v", err)
	}

	turns, err := turnMgr.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to list turns: %v", err)
	}

	if len(turns) != 3 {
		t.Errorf("expected 3 turns, got %d", len(turns))
	}
}

func TestTurnManager_Bead_l3d_1_4_RejectsSecondActiveMutableTurn(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	firstTurn, err := turnMgr.Create(ctx, sess.ID, "first message")
	if err != nil {
		t.Fatalf("failed to create first turn: %v", err)
	}

	_, err = turnMgr.Create(ctx, sess.ID, "second message")
	if err == nil {
		t.Fatal("expected active mutable turn conflict error")
	}
	if !errors.Is(err, ErrActiveMutableTurnConflict) {
		t.Fatalf("expected ErrActiveMutableTurnConflict, got %v", err)
	}

	storedTurn, err := turnMgr.Get(ctx, firstTurn.ID)
	if err != nil {
		t.Fatalf("expected read-only turn retrieval to work, got %v", err)
	}
	if storedTurn.ID != firstTurn.ID {
		t.Fatalf("expected retrieved turn %s, got %s", firstTurn.ID, storedTurn.ID)
	}

	turns, err := turnMgr.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("expected read-only listing to work, got %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected exactly one turn after conflict rejection, got %d", len(turns))
	}
}

func TestTurnManager_Bead_l3d_1_4_AllowsNewTurnAfterTerminalTransition(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	firstTurn, err := turnMgr.Create(ctx, sess.ID, "first message")
	if err != nil {
		t.Fatalf("failed to create first turn: %v", err)
	}

	payload, err := json.Marshal(map[string]string{"reason": "completed"})
	if err != nil {
		t.Fatalf("failed to marshal terminal transition payload: %v", err)
	}
	if err := eventStore.Append(ctx, audit.Event{
		ID:        uuid.New().String(),
		SessionID: firstTurn.SessionID,
		TurnID:    firstTurn.ID,
		EventType: "turn_completed",
		Timestamp: time.Now(),
		Payload:   payload,
	}); err != nil {
		t.Fatalf("failed to append terminal transition event: %v", err)
	}

	_, err = turnMgr.Create(ctx, sess.ID, "second message")
	if err != nil {
		t.Fatalf("expected second turn creation after completion, got %v", err)
	}
}

func TestContextAssembler_AddFragment(t *testing.T) {
	assembler := NewContextAssemblerWithPolicy(1000, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	fragment := &ContextFragment{
		SourceType:      "file",
		SourceRef:       "src/auth/service.go",
		Content:         "package auth...",
		TokenCount:      50,
		Truncated:       false,
		InclusionReason: "user_requested",
	}

	err := assembler.AddFragment(fragment)
	if err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	if len(assembler.fragments) != 1 {
		t.Errorf("expected 1 fragment, got %d", len(assembler.fragments))
	}
}

func appendTurnCompletedEvent(ctx context.Context, eventStore *audit.EventStore, sessionID, turnID string) error {
	return eventStore.Append(ctx, audit.Event{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		TurnID:    turnID,
		EventType: "turn_completed",
		Timestamp: time.Now(),
	})
}

func TestContextAssembler_TokenBudget(t *testing.T) {
	assembler := NewContextAssemblerWithPolicy(100, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	err := assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "file1.go",
		Content:    "content1",
		TokenCount: 60,
		Truncated:  false,
	})
	if err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	err = assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "file2.go",
		Content:    "content2",
		TokenCount: 60,
		Truncated:  false,
	})
	if err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	fragments := assembler.Fragments()
	if len(fragments) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(fragments))
	}

	if !fragments[1].Truncated {
		t.Error("expected second fragment to be truncated")
	}
}

func TestContextAssembler_BuildPrompt(t *testing.T) {
	assembler := NewContextAssemblerWithPolicy(1000, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	_ = assembler.AddFragment(&ContextFragment{
		SourceType: "instruction",
		SourceRef:  "system",
		Content:    "You are a helpful coding assistant.",
		TokenCount: 10,
		Truncated:  false,
	})

	_ = assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "src/main.go",
		Content:    "package main",
		TokenCount: 5,
		Truncated:  false,
	})

	prompt := assembler.BuildPrompt()

	if len(prompt) == 0 {
		t.Error("expected non-empty prompt")
	}
}

func TestContextAssembler_BuildPromptAppliesModelProviderPolicy(t *testing.T) {
	assembler := NewContextAssemblerWithPolicy(1000, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	_ = assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "src/keys.txt",
		Content:    "AWS key AKIA1234567890ABCDEF",
		TokenCount: 12,
	})

	prompt := assembler.BuildPrompt()

	if strings.Contains(prompt, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected prompt egress policy to remove raw secret")
	}
}

func TestContextAssembler_BuildPromptTracksDeniedFragmentState(t *testing.T) {
	assembler := NewContextAssemblerWithPolicy(1000, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})
	fragment := &ContextFragment{
		SourceType: "file",
		SourceRef:  "src/keys.txt",
		Content:    "AWS key AKIA1234567890ABCDEF",
		TokenCount: 12,
	}

	_ = assembler.AddFragment(fragment)
	prompt := assembler.BuildPrompt()

	if prompt != "" {
		t.Fatalf("expected denied fragment to be excluded from prompt")
	}
	if fragment.Included {
		t.Fatalf("expected denied fragment to be marked excluded")
	}
	if fragment.ExclusionReason != "policy_denied" {
		t.Fatalf("expected policy_denied exclusion reason, got %s", fragment.ExclusionReason)
	}
	if fragment.RedactionAction != "deny" {
		t.Fatalf("expected redaction action deny, got %s", fragment.RedactionAction)
	}
	if fragment.Sensitivity != "A" {
		t.Fatalf("expected sensitivity class A, got %s", fragment.Sensitivity)
	}
}

func TestContextAssembler_DefaultPolicyDeniesWhenNotConfigured(t *testing.T) {
	assembler := NewContextAssembler(1000)
	fragment := &ContextFragment{
		SourceType: "file",
		SourceRef:  "src/main.go",
		Content:    "package main",
		TokenCount: 5,
	}

	_ = assembler.AddFragment(fragment)
	prompt := assembler.BuildPrompt()

	if prompt != "" {
		t.Fatalf("expected secure default policy to deny prompt content")
	}
	if fragment.ExclusionReason != "policy_denied" {
		t.Fatalf("expected policy_denied exclusion reason, got %s", fragment.ExclusionReason)
	}
}

func TestTurnManager_CreatePersistsRedactionMetadata(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithPolicy(eventStore, sessionMgr, secretsRedactionPolicy{executor: secrets.NewPolicyExecutor(nil)})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "my key is AKIA1234567890ABCDEF")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}
	if strings.Contains(turn.Message, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected persisted turn message to be sanitized")
	}

	events, err := eventStore.QueryBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	var payload map[string]interface{}
	for _, event := range events {
		if event.EventType != "turn_started" {
			continue
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
	}

	if payload["redaction_action"] != "hash" {
		t.Fatalf("expected redaction_action hash, got %v", payload["redaction_action"])
	}
	if payload["sensitivity_class"] != "A" {
		t.Fatalf("expected sensitivity_class A, got %v", payload["sensitivity_class"])
	}
	if payload["redaction_denied"] != false {
		t.Fatalf("expected redaction_denied false, got %v", payload["redaction_denied"])
	}
	if payload["entity_type"] != "turn" {
		t.Fatalf("expected entity_type turn, got %v", payload["entity_type"])
	}
	if payload["entity_id"] != turn.ID {
		t.Fatalf("expected entity_id %s, got %v", turn.ID, payload["entity_id"])
	}
	if payload["previous_state"] != "none" {
		t.Fatalf("expected previous_state none, got %v", payload["previous_state"])
	}
	if payload["next_state"] != string(TurnStatusActive) {
		t.Fatalf("expected next_state active, got %v", payload["next_state"])
	}
	if payload["cause"] != "turn_create_requested" {
		t.Fatalf("expected cause turn_create_requested, got %v", payload["cause"])
	}
}

func TestContextManifest_RecordPreservesRedactionState(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manifest := NewContextManifest(eventStore)
	ctx := context.Background()

	fragments := []ContextFragment{
		{
			SourceType:      "file",
			SourceRef:       "secrets.txt",
			Content:         "[DENIED]",
			TokenCount:      12,
			Included:        false,
			ExclusionReason: "policy_denied",
			Sensitivity:     "A",
			RedactionAction: "deny",
			RedactionDenied: true,
			RedactionReason: "A policy for model_provider destination",
		},
	}

	if err := manifest.Record(ctx, "s1", "t1", fragments); err != nil {
		t.Fatalf("failed to record manifest: %v", err)
	}

	retrieved, err := manifest.Get(ctx, "t1")
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	if len(retrieved) != 1 {
		t.Fatalf("expected one fragment, got %d", len(retrieved))
	}
	if retrieved[0].RedactionAction != "deny" {
		t.Fatalf("expected redaction action to roundtrip, got %s", retrieved[0].RedactionAction)
	}
	if !retrieved[0].RedactionDenied {
		t.Fatalf("expected denied redaction flag to roundtrip")
	}
	if retrieved[0].ExclusionReason != "policy_denied" {
		t.Fatalf("expected exclusion reason to roundtrip, got %s", retrieved[0].ExclusionReason)
	}
}

func TestContextManifest_Record(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manifest := NewContextManifest(eventStore)

	ctx := context.Background()
	sessionID := "test-session"
	turnID := "test-turn"

	fragments := []ContextFragment{
		{
			SourceType:      "file",
			SourceRef:       "src/main.go",
			Content:         "package main",
			TokenCount:      100,
			Included:        true,
			Truncated:       false,
			InclusionReason: "user_requested",
			Sensitivity:     "none",
			FreshnessState:  "fresh",
			Conflicts: []ContextConflict{
				{WithSourceRef: "HEAD~1:src/main.go", Reason: "content_mismatch"},
			},
		},
		{
			SourceType:      "git",
			SourceRef:       "HEAD",
			Content:         "diff --git",
			TokenCount:      50,
			Included:        false,
			ExclusionReason: "lower_priority",
			Truncated:       false,
			InclusionReason: "auto",
			Sensitivity:     "internal",
			FreshnessState:  "stale",
		},
	}

	err = manifest.Record(ctx, sessionID, turnID, fragments)
	if err != nil {
		t.Fatalf("failed to record manifest: %v", err)
	}

	retrieved, err := manifest.Get(ctx, turnID)
	if err != nil {
		t.Fatalf("failed to get manifest: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(retrieved))
	}
	if !retrieved[0].Included {
		t.Fatal("expected first fragment to be included")
	}
	if retrieved[1].Included {
		t.Fatal("expected second fragment to be excluded")
	}
	if retrieved[1].ExclusionReason != "lower_priority" {
		t.Fatalf("unexpected exclusion reason: %s", retrieved[1].ExclusionReason)
	}
	if retrieved[0].FreshnessState != "fresh" {
		t.Fatalf("unexpected freshness state: %s", retrieved[0].FreshnessState)
	}
	if len(retrieved[0].Conflicts) != 1 {
		t.Fatalf("expected one conflict record, got %d", len(retrieved[0].Conflicts))
	}
}

func TestTokenBudget_Allocation(t *testing.T) {
	budget := NewTokenBudget(1000)

	err := budget.Allocate("system", 100)
	if err != nil {
		t.Fatalf("failed to allocate: %v", err)
	}
	err = budget.Allocate("context", 500)
	if err != nil {
		t.Fatalf("failed to allocate: %v", err)
	}
	err = budget.Allocate("user", 200)
	if err != nil {
		t.Fatalf("failed to allocate: %v", err)
	}

	if budget.Used() != 800 {
		t.Errorf("expected 800 used tokens, got %d", budget.Used())
	}

	if budget.Remaining() != 200 {
		t.Errorf("expected 200 remaining tokens, got %d", budget.Remaining())
	}

	err = budget.Allocate("extra", 300)
	if err == nil {
		t.Error("expected error for over-allocation")
	}
}
