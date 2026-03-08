package context

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func testPolicyRedactor(sourceRef, _ string, content string) RedactionDecision {
	if strings.Contains(content, "AKIA1234567890ABCDEF") {
		if sourceRef == "turn.message" {
			return RedactionDecision{
				Content:     "sha256:test-redaction",
				Action:      "hash",
				Sensitivity: "A",
			}
		}
		return RedactionDecision{
			Content:     "",
			Action:      "deny",
			Denied:      true,
			Reason:      "A policy for model_provider destination",
			Sensitivity: "A",
		}
	}

	return RedactionDecision{Content: content, Action: "none", Sensitivity: "none"}
}

func TestTurnManager_Create(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, testPolicyRedactor)

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
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, testPolicyRedactor)

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

func TestTurnManager_CreateUsesInjectedRedactor(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, func(_ string, _ string, content string) RedactionDecision {
		return RedactionDecision{
			Content:     "[redacted] " + content,
			Action:      "mask",
			Sensitivity: "A",
		}
	})

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "top secret")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if turn.Message != "[redacted] top secret" {
		t.Fatalf("expected injected redactor message, got %s", turn.Message)
	}
}

func TestNewTurnManager_DefaultRedactorHashesPersistedContent(t *testing.T) {
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

	turn, err := turnMgr.Create(ctx, sess.ID, "hello world")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if !strings.HasPrefix(turn.Message, "sha256:") {
		t.Fatalf("expected default constructor to hash persisted content, got %s", turn.Message)
	}
}

func TestNewTurnManagerWithRedactorNilUsesSafeDefault(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, nil)

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	turn, err := turnMgr.Create(ctx, sess.ID, "hello world")
	if err != nil {
		t.Fatalf("failed to create turn: %v", err)
	}

	if !strings.HasPrefix(turn.Message, "sha256:") {
		t.Fatalf("expected nil redactor to use safe hash default, got %s", turn.Message)
	}
}

func TestTurnManager_Get(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, testPolicyRedactor)

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
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, testPolicyRedactor)

	ctx := context.Background()

	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	_, _ = turnMgr.Create(ctx, sess.ID, "First message")
	_, _ = turnMgr.Create(ctx, sess.ID, "Second message")
	_, _ = turnMgr.Create(ctx, sess.ID, "Third message")

	turns, err := turnMgr.ListBySession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to list turns: %v", err)
	}

	if len(turns) != 3 {
		t.Errorf("expected 3 turns, got %d", len(turns))
	}
}

func TestContextAssembler_AddFragment(t *testing.T) {
	assembler := NewContextAssemblerWithRedactor(1000, testPolicyRedactor)

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

func TestContextAssembler_TokenBudget(t *testing.T) {
	assembler := NewContextAssembler(100)

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
	assembler := NewContextAssemblerWithRedactor(1000, testPolicyRedactor)

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
	assembler := NewContextAssemblerWithRedactor(1000, testPolicyRedactor)

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

func TestContextAssembler_BuildPromptUsesInjectedRedactor(t *testing.T) {
	assembler := NewContextAssemblerWithRedactor(1000, func(sourceRef string, sourceType string, content string) RedactionDecision {
		if sourceRef == "src/keys.txt" && sourceType == "file" {
			return RedactionDecision{
				Content:     "[safe]",
				Action:      "mask",
				Sensitivity: "A",
			}
		}
		return RedactionDecision{Content: content, Action: "none", Sensitivity: "none"}
	})

	fragment := &ContextFragment{
		SourceType: "file",
		SourceRef:  "src/keys.txt",
		Content:    "AWS key AKIA1234567890ABCDEF",
		TokenCount: 12,
	}
	_ = assembler.AddFragment(fragment)

	prompt := assembler.BuildPrompt()

	if prompt != "[safe]" {
		t.Fatalf("expected injected redacted prompt, got %s", prompt)
	}
	if fragment.RedactionAction != "mask" {
		t.Fatalf("expected redaction action mask, got %s", fragment.RedactionAction)
	}
}

func TestNewContextAssembler_DefaultRedactorDeniesPromptByDefault(t *testing.T) {
	assembler := NewContextAssembler(1000)
	fragment := &ContextFragment{
		SourceType: "instruction",
		SourceRef:  "system",
		Content:    "do work",
		TokenCount: 5,
	}

	if err := assembler.AddFragment(fragment); err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	prompt := assembler.BuildPrompt()
	if prompt != "" {
		t.Fatalf("expected default model-provider safeguard to deny prompt content")
	}
	if !fragment.RedactionDenied {
		t.Fatalf("expected denied flag for default safeguard")
	}
}

func TestNewContextAssemblerWithRedactorNilUsesSafeDefault(t *testing.T) {
	assembler := NewContextAssemblerWithRedactor(1000, nil)
	fragment := &ContextFragment{
		SourceType: "instruction",
		SourceRef:  "system",
		Content:    "do work",
		TokenCount: 5,
	}

	if err := assembler.AddFragment(fragment); err != nil {
		t.Fatalf("failed to add fragment: %v", err)
	}

	prompt := assembler.BuildPrompt()
	if prompt != "" {
		t.Fatalf("expected nil redactor to use deny safeguard")
	}
}

func TestContextAssembler_BuildPromptTracksDeniedFragmentState(t *testing.T) {
	assembler := NewContextAssemblerWithRedactor(1000, testPolicyRedactor)
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

func TestTurnManager_CreatePersistsRedactionMetadata(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManagerWithRedactor(eventStore, sessionMgr, testPolicyRedactor)

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
