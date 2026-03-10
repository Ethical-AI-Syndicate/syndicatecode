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

func TestHandleSessionByID_EventsCanBeFilteredByType(t *testing.T) {
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
		EventType: "mcp.call",
		Actor:     "controlplane",
		Payload:   json.RawMessage(`{"server_id":"inventory.remote"}`),
	}); err != nil {
		t.Fatalf("failed to append mcp.call event: %v", err)
	}

	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        uuid.NewString(),
		SessionID: created.ID,
		Timestamp: time.Now().UTC(),
		EventType: "tool_output_redaction",
		Actor:     "system",
		Payload:   json.RawMessage(`{"notice_count":1}`),
	}); err != nil {
		t.Fatalf("failed to append tool_output_redaction event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=mcp.call", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode filtered events: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
	if events[0].EventType != "mcp.call" {
		t.Fatalf("expected mcp.call event, got %s", events[0].EventType)
	}
}

func TestHandleSessionByID_EventsFilterWithNoMatchesReturnsEmptyList(t *testing.T) {
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

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=mcp.call", nil)
	rec := httptest.NewRecorder()

	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode empty filtered events response: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected empty filtered event list, got %d entries", len(events))
	}
}

func TestHandleSessionByID_EventsDeterministicOrder_Bead_l3d_2_2(t *testing.T) {
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

	timestamp := time.Now().UTC().Add(50 * time.Millisecond).Truncate(time.Second)
	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        "b-event",
		SessionID: created.ID,
		Timestamp: timestamp,
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"seq":2}`),
	}); err != nil {
		t.Fatalf("failed to append first event: %v", err)
	}
	if err := eventStore.Append(context.Background(), audit.Event{
		ID:        "a-event",
		SessionID: created.ID,
		Timestamp: timestamp,
		EventType: "turn_completed",
		Actor:     "system",
		Payload:   json.RawMessage(`{"seq":1}`),
	}); err != nil {
		t.Fatalf("failed to append second event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/events?event_type=turn_completed", nil)
	rec := httptest.NewRecorder()
	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []audit.Event
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 filtered events, got %d", len(events))
	}

	if events[0].ID != "a-event" || events[1].ID != "b-event" {
		t.Fatalf("expected deterministic ordering by (timestamp,id): [a-event b-event], got [%s %s]", events[0].ID, events[1].ID)
	}
}
