package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func TestHandleEventTypes_ReturnsKnownTypes(t *testing.T) {
	server := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/types", nil)
	rec := httptest.NewRecorder()

	server.handleEventTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		EventTypes []string `json:"event_types"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode event types response: %v", err)
	}
	if len(payload.EventTypes) == 0 {
		t.Fatal("expected non-empty event_types response")
	}
	expected := audit.KnownEventTypes()
	if len(payload.EventTypes) != len(expected) {
		t.Fatalf("expected %d event types, got %d", len(expected), len(payload.EventTypes))
	}
	for i := range expected {
		if payload.EventTypes[i] != expected[i] {
			t.Fatalf("expected event_types[%d]=%q, got %q", i, expected[i], payload.EventTypes[i])
		}
	}
	if !audit.IsKnownEventType(payload.EventTypes[0]) {
		t.Fatalf("expected %q to be recognized event type", payload.EventTypes[0])
	}
}

func TestEventsTypesEndpoint_RequiresBearerTokenWhenConfigured(t *testing.T) {
	server := &Server{authToken: "secret-token"}
	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/types", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
