package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestHandleSessionByID_ExportPipeline_Bead_l3d_10_2(t *testing.T) {
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
		EventType: "tool_output",
		Actor:     "system",
		Payload:   json.RawMessage(`{"value":"AKIA1234567890ABCDEF"}`),
	}); err != nil {
		t.Fatalf("failed to append replay event: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, eventStore: eventStore}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID+"/export", nil)
	rec := httptest.NewRecorder()
	server.handleSessionByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Events []struct {
			Payload map[string]interface{} `json:"payload"`
		} `json:"events"`
		Warnings []struct {
			Destination    string `json:"destination"`
			MaterialImpact bool   `json:"material_impact"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode export response: %v", err)
	}

	if len(resp.Warnings) == 0 {
		t.Fatalf("expected export warnings for redacted payload")
	}
	if resp.Warnings[0].Destination != "export" {
		t.Fatalf("expected export warning destination, got %q", resp.Warnings[0].Destination)
	}
	if !resp.Warnings[0].MaterialImpact {
		t.Fatalf("expected material impact warning")
	}

	value, _ := resp.Events[1].Payload["value"].(string)
	if strings.Contains(value, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected export payload to be filtered, got %q", value)
	}
}
