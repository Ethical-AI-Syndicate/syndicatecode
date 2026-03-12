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

func TestAPIClient_GetEventTypes_UsesWrappedResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/events/types" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"event_types":["mcp.call","tool_result"]}`))
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	types, err := client.GetEventTypes(context.Background())
	if err != nil {
		t.Fatalf("get event types failed: %v", err)
	}
	if len(types) != 2 || types[0] != "mcp.call" {
		t.Fatalf("unexpected event types: %+v", types)
	}
}

func TestAPIClient_GetDiagnostics_DecodesResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lsp/diagnostics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"diagnostics":[{"path":"main.go","severity":"error","message":"undefined","range":{"start_line":1,"start_col":1,"end_line":1,"end_col":4}}]}`))
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	items, err := client.GetDiagnostics(context.Background(), "s-1", "main.go")
	if err != nil {
		t.Fatalf("get diagnostics failed: %v", err)
	}
	if len(items) != 1 || items[0].Range.EndCol != 4 {
		t.Fatalf("unexpected diagnostics: %+v", items)
	}
}

func TestAPIClient_GetHoverAndDefinition_DecodeResponses(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/lsp/hover":
			_, _ = w.Write([]byte(`{"contents":"func main()","range":{"start_line":1,"start_col":1,"end_line":1,"end_col":4}}`))
		case "/api/v1/lsp/definition":
			_, _ = w.Write([]byte(`{"locations":[{"path":"main.go","range":{"start_line":10,"start_col":1,"end_line":10,"end_col":4}}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	client := NewAPIClient(ts.URL)
	hover, err := client.GetHover(context.Background(), LSPPositionRequest{SessionID: "s-1", Path: "main.go", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("get hover failed: %v", err)
	}
	if hover == nil || hover.Contents == "" {
		t.Fatalf("unexpected hover: %+v", hover)
	}

	locs, err := client.GetDefinition(context.Background(), LSPPositionRequest{SessionID: "s-1", Path: "main.go", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("get definition failed: %v", err)
	}
	if len(locs) != 1 || locs[0].Path != "main.go" {
		t.Fatalf("unexpected definition locations: %+v", locs)
	}
}
