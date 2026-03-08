package context

import (
	"context"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestTurnManager_Create(t *testing.T) {
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
	turnMgr := NewTurnManager(eventStore, sessionMgr)

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
}

func TestTurnManager_Get(t *testing.T) {
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
	turnMgr := NewTurnManager(eventStore, sessionMgr)

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
	assembler := NewContextAssembler(1000)

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
	assembler := NewContextAssembler(1000)

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
