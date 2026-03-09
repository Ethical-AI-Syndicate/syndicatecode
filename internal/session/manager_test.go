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
