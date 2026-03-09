package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	ctxmgr "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/context"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestCreateTurn_Bead_l3d_1_4_RejectsConcurrentMutableTurnsAndPreservesReadOnlyAccess(t *testing.T) {
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
	server.createTurn(first, httptest.NewRequest(http.MethodPost, "/api/v1/turns", firstBody))
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
	server.createTurn(second, httptest.NewRequest(http.MethodPost, "/api/v1/turns", secondBody))
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
