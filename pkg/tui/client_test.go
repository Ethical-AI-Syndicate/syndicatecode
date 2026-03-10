package tui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIClient_ListSessions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"s1","repo_path":"/repo","trust_tier":"tier1","status":"active"}]`))
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	sessions, err := client.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}
}

func TestAPIClient_ErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	_, err := client.ListApprovals(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAPIClient_GetPolicy_Bead_l3d_15_4(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/policy" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1.0.0"}`))
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	policy, err := client.GetPolicy(context.Background())
	if err != nil {
		t.Fatalf("get policy failed: %v", err)
	}
	if policy["version"] != "1.0.0" {
		t.Fatalf("unexpected policy: %+v", policy)
	}
}

func TestAPIClient_ListSessionEvents_Bead_l3d_15_4(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions/s-1/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("event_type") != "mcp.call" {
			t.Fatalf("unexpected event_type: %s", r.URL.Query().Get("event_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"e1","session_id":"s-1","timestamp":"2026-03-10T00:00:00Z","event_type":"mcp.call","actor":"controlplane","payload":{}}]`))
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	events, err := client.ListSessionEvents(context.Background(), "s-1", "mcp.call")
	if err != nil {
		t.Fatalf("list session events failed: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "mcp.call" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
