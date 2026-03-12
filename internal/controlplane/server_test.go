package controlplane

import (
	"bytes"
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
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestCreateTurn_RejectsConcurrentMutableTurnsAndPreservesReadOnlyAccess_Bead_l3d_1_4(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	turnMgr := ctxmgr.NewTurnManager(eventStore, sessionMgr)
	server := &Server{turnMgr: turnMgr}

	ctx := context.Background()
	sess, err := sessionMgr.Create(ctx, "/repo", "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	first := httptest.NewRecorder()
	firstBody := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","message":"first"}`)
	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/turns", firstBody)
	server.createTurn(first, withOperatorRole(firstReq))
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first create status %d, got %d", http.StatusCreated, first.Code)
	}

	var firstTurn struct {
		ID string `json:"turn_id"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstTurn); err != nil {
		t.Fatalf("failed to decode first turn response: %v", err)
	}
	if firstTurn.ID == "" {
		t.Fatal("expected first turn id")
	}

	second := httptest.NewRecorder()
	secondBody := bytes.NewBufferString(`{"session_id":"` + sess.ID + `","message":"second"}`)
	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/turns", secondBody)
	server.createTurn(second, withOperatorRole(secondReq))
	if second.Code != http.StatusConflict {
		t.Fatalf("expected second create status %d, got %d", http.StatusConflict, second.Code)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/turns?session_id="+sess.ID, nil)
	server.listTurns(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, list.Code)
	}

	var turns []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &turns); err != nil {
		t.Fatalf("failed to decode turns list response: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected one persisted turn after conflict, got %d", len(turns))
	}

	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/turns/"+firstTurn.ID, nil)
	server.handleTurnByID(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("expected turn get status %d, got %d", http.StatusOK, get.Code)
	}
}

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
			EventType string          `json:"event_type"`
			Payload   json.RawMessage `json:"payload"`
		} `json:"events"`
		RedactionSummary struct {
			Reason string `json:"reason"`
		} `json:"redaction_summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode export response: %v", err)
	}

	if resp.RedactionSummary.Reason == "" {
		t.Fatalf("expected non-empty redaction_summary.reason for redacted payload")
	}

	var valuePayload map[string]interface{}
	for _, e := range resp.Events {
		if e.EventType == "tool_output" {
			if err := json.Unmarshal(e.Payload, &valuePayload); err != nil {
				t.Fatalf("failed to decode event payload: %v", err)
			}
			break
		}
	}
	value, _ := valuePayload["value"].(string)
	if strings.Contains(value, "AKIA1234567890ABCDEF") {
		t.Fatalf("expected export payload to be filtered, got %q", value)
	}
}
