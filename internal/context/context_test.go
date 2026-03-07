package context

import (
	"context"
	"testing"

	"github.com/syndicatecode/syndicatecode/internal/audit"
	"github.com/syndicatecode/syndicatecode/internal/session"
)

func TestTurnManager_Create(t *testing.T) {
	// Setup
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManager(eventStore, sessionMgr)

	ctx := context.Background()

	// Create a session first
	sess, err := sessionMgr.Create(ctx, "/test/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create a turn
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

func TestTurnManager_Get(t *testing.T) {
	// Setup
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManager(eventStore, sessionMgr)

	ctx := context.Background()

	// Create session and turn
	sess, _ := sessionMgr.Create(ctx, "/test/repo", "tier1")
	created, _ := turnMgr.Create(ctx, sess.ID, "Test message")

	// Get the turn
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
	// Setup
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer eventStore.Close()

	sessionMgr := session.NewManager(eventStore)
	turnMgr := NewTurnManager(eventStore, sessionMgr)

	ctx := context.Background()

	// Create session
	sess, _ := sessionMgr.Create(ctx, "/test/repo", "tier1")

	// Create multiple turns
	turnMgr.Create(ctx, sess.ID, "First message")
	turnMgr.Create(ctx, sess.ID, "Second message")
	turnMgr.Create(ctx, sess.ID, "Third message")

	// List turns
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
		SourceType:     "file",
		SourceRef:      "src/auth/service.go",
		Content:        "package auth...",
		TokenCount:     50,
		Truncated:      false,
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

	// Add fragments that exceed budget
	assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "file1.go",
		Content:     "content1",
		TokenCount: 60,
		Truncated:  false,
	})

	assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "file2.go",
		Content:     "content2",
		TokenCount: 60,
		Truncated:  false,
	})

	// The second fragment should be truncated
	fragments := assembler.Fragments()
	if len(fragments) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(fragments))
	}

	// Second fragment should be truncated due to budget
	if !fragments[1].Truncated {
		t.Error("expected second fragment to be truncated")
	}
}

func TestContextAssembler_BuildPrompt(t *testing.T) {
	assembler := NewContextAssembler(1000)

	assembler.AddFragment(&ContextFragment{
		SourceType: "instruction",
		SourceRef:  "system",
		Content:     "You are a helpful coding assistant.",
		TokenCount:  10,
		Truncated:   false,
	})

	assembler.AddFragment(&ContextFragment{
		SourceType: "file",
		SourceRef:  "src/main.go",
		Content:     "package main",
		TokenCount:  5,
		Truncated:   false,
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
	defer eventStore.Close()

	manifest := NewContextManifest(eventStore)

	ctx := context.Background()
	sessionID := "test-session"
	turnID := "test-turn"

	// Record fragments
	fragments := []ContextFragment{
		{
			SourceType:      "file",
			SourceRef:       "src/main.go",
			TokenCount:      100,
			Truncated:       false,
			InclusionReason: "user_requested",
		},
		{
			SourceType:      "git",
			SourceRef:       "HEAD",
			TokenCount:      50,
			Truncated:       false,
			InclusionReason: "auto",
		},
	}

	err = manifest.Record(ctx, sessionID, turnID, fragments)
	if err != nil {
		t.Fatalf("failed to record manifest: %v", err)
	}

	// Query manifest
	retrieved, err := manifest.Get(ctx, turnID)
	if err != nil {
		t.Fatalf("failed to get manifest: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("expected 2 fragments, got %d", len(retrieved))
	}
}

func TestTokenBudget_Allocation(t *testing.T) {
	budget := NewTokenBudget(1000)

	// Allocate tokens
	budget.Allocate("system", 100)
	budget.Allocate("context", 500)
	budget.Allocate("user", 200)

	if budget.Used() != 800 {
		t.Errorf("expected 800 used tokens, got %d", budget.Used())
	}

	if budget.Remaining() != 200 {
		t.Errorf("expected 200 remaining tokens, got %d", budget.Remaining())
	}

	// Should not allow over-allocation
	err := budget.Allocate("extra", 300)
	if err == nil {
		t.Error("expected error for over-allocation")
	}
}
