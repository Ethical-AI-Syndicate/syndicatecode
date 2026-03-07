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
