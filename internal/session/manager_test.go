package session

import (
	"context"
	"encoding/json"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func TestManager_CreateEmitsTransitionCausalityPayload(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manager := NewManager(eventStore)
	created, err := manager.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	events, err := eventStore.QueryBySession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].EventType != "session_started" {
		t.Fatalf("expected session_started event, got %s", events[0].EventType)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	if payload["entity_type"] != "session" {
		t.Fatalf("expected entity_type session, got %v", payload["entity_type"])
	}
	if payload["entity_id"] != created.ID {
		t.Fatalf("expected entity_id %s, got %v", created.ID, payload["entity_id"])
	}
	if payload["previous_state"] != "none" {
		t.Fatalf("expected previous_state none, got %v", payload["previous_state"])
	}
	if payload["next_state"] != string(StatusActive) {
		t.Fatalf("expected next_state active, got %v", payload["next_state"])
	}
	if payload["cause"] != "session_create_requested" {
		t.Fatalf("expected cause session_create_requested, got %v", payload["cause"])
	}
}

func TestManager_DeleteSoftHidesSession_Bead_l3d_10_3(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manager := NewManager(eventStore)
	sessionObj, err := manager.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := manager.Delete(context.Background(), sessionObj.ID, DeleteModeSoft, "user_hidden"); err != nil {
		t.Fatalf("soft delete failed: %v", err)
	}

	if _, err := manager.Get(context.Background(), sessionObj.ID); err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound after soft delete, got %v", err)
	}

	listed, err := manager.List(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no listed sessions after soft delete, got %d", len(listed))
	}
}

func TestManager_DeleteHardRemovesSessionArtifacts_Bead_l3d_10_3(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manager := NewManager(eventStore)
	sessionObj, err := manager.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := manager.Delete(context.Background(), sessionObj.ID, DeleteModeHard, "user_purge"); err != nil {
		t.Fatalf("hard delete failed: %v", err)
	}

	events, err := eventStore.QueryBySession(context.Background(), sessionObj.ID)
	if err != nil {
		t.Fatalf("query session events failed: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no retained events after hard delete, got %d", len(events))
	}
}

func TestManager_DeleteTombstoneKeepsMinimalAudit_Bead_l3d_10_3(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	manager := NewManager(eventStore)
	sessionObj, err := manager.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := manager.Delete(context.Background(), sessionObj.ID, DeleteModeTombstone, "policy_required"); err != nil {
		t.Fatalf("tombstone delete failed: %v", err)
	}

	events, err := eventStore.QueryBySession(context.Background(), sessionObj.ID)
	if err != nil {
		t.Fatalf("query session events failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one minimal tombstone event, got %d", len(events))
	}
	if events[0].EventType != "session_tombstoned" {
		t.Fatalf("expected session_tombstoned event, got %s", events[0].EventType)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("failed to decode tombstone payload: %v", err)
	}
	if payload["reason"] != "policy_required" {
		t.Fatalf("expected policy_required reason, got %v", payload["reason"])
	}
	if payload["mode"] != string(DeleteModeTombstone) {
		t.Fatalf("expected mode %s, got %v", DeleteModeTombstone, payload["mode"])
	}
}
