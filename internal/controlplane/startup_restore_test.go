package controlplane

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestRestoreRuntimeStateRestoresPendingApprovals(t *testing.T) {
	store, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	approval := Approval{
		ID:            "apr-1",
		SessionID:     "s-1",
		ToolName:      "write_file",
		ArgumentsHash: "hash",
		SideEffect:    tools.SideEffectWrite,
		State:         ApprovalStatePending,
		CreatedAt:     time.Now().UTC().Add(-1 * time.Minute),
		UpdatedAt:     time.Now().UTC().Add(-1 * time.Minute),
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
		Call: tools.ToolCall{
			ToolName: "write_file",
			Input:    map[string]interface{}{"path": "x.txt", "content": "hello"},
		},
	}

	payload, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("failed to marshal approval payload: %v", err)
	}

	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: approval.SessionID,
		Timestamp: time.Now().UTC(),
		EventType: "approval_proposed",
		Actor:     "system",
		Payload:   payload,
	}); err != nil {
		t.Fatalf("failed to append approval_proposed event: %v", err)
	}

	server := &Server{
		eventStore:  store,
		approvalMgr: NewApprovalManager(15 * time.Minute),
	}

	if err := server.restoreRuntimeState(context.Background()); err != nil {
		t.Fatalf("restoreRuntimeState returned error: %v", err)
	}

	restored, ok := server.approvalMgr.Get(approval.ID)
	if !ok {
		t.Fatalf("expected restored approval %s to exist", approval.ID)
	}
	if restored.State != ApprovalStatePending {
		t.Fatalf("expected restored state %s, got %s", ApprovalStatePending, restored.State)
	}
}

func TestRestoreRuntimeStateCancelsInterruptedApprovedApprovals(t *testing.T) {
	store, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	approval := Approval{
		ID:            "apr-2",
		SessionID:     "s-1",
		ToolName:      "write_file",
		ArgumentsHash: "hash",
		SideEffect:    tools.SideEffectWrite,
		State:         ApprovalStatePending,
		CreatedAt:     time.Now().UTC().Add(-2 * time.Minute),
		UpdatedAt:     time.Now().UTC().Add(-2 * time.Minute),
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
		Call: tools.ToolCall{
			ToolName: "write_file",
			Input:    map[string]interface{}{"path": "x.txt", "content": "hello"},
		},
	}

	proposedPayload, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("failed to marshal approval payload: %v", err)
	}

	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: approval.SessionID,
		Timestamp: time.Now().UTC().Add(-90 * time.Second),
		EventType: "approval_proposed",
		Actor:     "system",
		Payload:   proposedPayload,
	}); err != nil {
		t.Fatalf("failed to append approval_proposed event: %v", err)
	}

	decisionPayload, err := json.Marshal(map[string]string{
		"approval_id": approval.ID,
		"decision":    "approve",
	})
	if err != nil {
		t.Fatalf("failed to marshal decision payload: %v", err)
	}

	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: approval.SessionID,
		Timestamp: time.Now().UTC().Add(-60 * time.Second),
		EventType: "approval_decided",
		Actor:     "system",
		Payload:   decisionPayload,
	}); err != nil {
		t.Fatalf("failed to append approval_decided event: %v", err)
	}

	server := &Server{
		eventStore:  store,
		approvalMgr: NewApprovalManager(15 * time.Minute),
	}

	if err := server.restoreRuntimeState(context.Background()); err != nil {
		t.Fatalf("restoreRuntimeState returned error: %v", err)
	}

	restored, ok := server.approvalMgr.Get(approval.ID)
	if !ok {
		t.Fatalf("expected restored approval %s to exist", approval.ID)
	}
	if restored.State != ApprovalStateCancelled {
		t.Fatalf("expected restored state %s, got %s", ApprovalStateCancelled, restored.State)
	}
}

func TestRestoreRuntimeState_RequiresOperatorReentryForInterruptedRiskyApproval_Bead_l3d_1_3(t *testing.T) {
	store, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	defer func() {
		_ = store.Close()
	}()

	approval := Approval{
		ID:            "apr-3",
		SessionID:     "s-1",
		ToolName:      "write_file",
		ArgumentsHash: "hash",
		SideEffect:    tools.SideEffectWrite,
		State:         ApprovalStatePending,
		CreatedAt:     time.Now().UTC().Add(-2 * time.Minute),
		UpdatedAt:     time.Now().UTC().Add(-2 * time.Minute),
		ExpiresAt:     time.Now().UTC().Add(10 * time.Minute),
		Call: tools.ToolCall{
			ToolName: "write_file",
			Input:    map[string]interface{}{"path": "x.txt", "content": "hello"},
		},
	}

	proposedPayload, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("failed to marshal approval payload: %v", err)
	}

	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: approval.SessionID,
		Timestamp: time.Now().UTC().Add(-90 * time.Second),
		EventType: "approval_proposed",
		Actor:     "system",
		Payload:   proposedPayload,
	}); err != nil {
		t.Fatalf("failed to append approval_proposed event: %v", err)
	}

	decisionPayload, err := json.Marshal(map[string]string{
		"approval_id": approval.ID,
		"decision":    "approve",
	})
	if err != nil {
		t.Fatalf("failed to marshal decision payload: %v", err)
	}

	if err := store.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: approval.SessionID,
		Timestamp: time.Now().UTC().Add(-60 * time.Second),
		EventType: "approval_decided",
		Actor:     "system",
		Payload:   decisionPayload,
	}); err != nil {
		t.Fatalf("failed to append approval_decided event: %v", err)
	}

	server := &Server{
		eventStore:  store,
		approvalMgr: NewApprovalManager(15 * time.Minute),
	}

	if err := server.restoreRuntimeState(context.Background()); err != nil {
		t.Fatalf("restoreRuntimeState returned error: %v", err)
	}

	restored, ok := server.approvalMgr.Get(approval.ID)
	if !ok {
		t.Fatalf("expected restored approval %s to exist", approval.ID)
	}
	if restored.State != ApprovalStateCancelled {
		t.Fatalf("expected restored state %s, got %s", ApprovalStateCancelled, restored.State)
	}
	if restored.DecisionReason != "operator re-entry required after startup recovery" {
		t.Fatalf("expected explicit operator re-entry reason, got %q", restored.DecisionReason)
	}
}
