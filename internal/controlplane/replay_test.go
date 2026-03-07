package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestHandleSessionByID_EventsReturnsReplayStream(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), "/tmp/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"turn_id":"t1"}`),
	}); err != nil {
		t.Fatalf("failed to append replay event: %v", err)
	}

	server := &Server{
		sessionMgr: sessionMgr,
		eventStore: eventStore,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode replay events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events in replay stream, got %d", len(events))
	}
	if events[0].EventType != "session_started" {
		t.Fatalf("expected first event to be session_started, got %s", events[0].EventType)
	}
	if events[1].EventType != "turn_completed" {
		t.Fatalf("expected second event to be turn_completed, got %s", events[1].EventType)
	}
}

func TestHandleSessionByID_EventsUnknownSessionReturnsNotFound(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	server := &Server{eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/missing/events", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
