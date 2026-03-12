package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestTools_SymbolicShellRegistered_Bead_l3d_X_1(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	registry, _, err := initializeTooling(context.Background(), eventStore, sessionMgr)
	if err != nil {
		t.Fatalf("initialize tooling: %v", err)
	}

	server := &Server{toolRegistry: registry}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tools", nil)
	rr := httptest.NewRecorder()

	server.handleTools(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var payload struct {
		Tools []tools.ToolDefinition `json:"tools"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	found := false
	for _, def := range payload.Tools {
		if def.Name == "symbolic_shell" {
			if def.TrustLevel != "tier1" {
				t.Fatalf("expected symbolic_shell trust_level=tier1, got %q", def.TrustLevel)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected symbolic_shell to be registered")
	}
}

func TestHandleToolExecute_SymbolicShellVisibilityHonorsTrustLevel(t *testing.T) {
	eventStore, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	registry, executor, err := initializeTooling(context.Background(), eventStore, sessionMgr)
	if err != nil {
		t.Fatalf("initialize tooling: %v", err)
	}

	tier1Session, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("create tier1 session: %v", err)
	}
	tier2Session, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier2")
	if err != nil {
		t.Fatalf("create tier2 session: %v", err)
	}

	server := &Server{
		sessionMgr:   sessionMgr,
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
		eventStore:   eventStore,
		httpServer:   &http.Server{},
	}

	tier1Body := bytes.NewBufferString(`{"session_id":"` + tier1Session.ID + `","tool_name":"symbolic_shell","input":{"command":"go_version"}}`)
	tier1Req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", tier1Body)
	tier1Rec := httptest.NewRecorder()
	server.handleToolExecute(tier1Rec, tier1Req)
	if tier1Rec.Code != http.StatusAccepted {
		t.Fatalf("expected symbolic_shell to be visible for tier1 session (202), got %d: %s", tier1Rec.Code, tier1Rec.Body.String())
	}

	tier2Body := bytes.NewBufferString(`{"session_id":"` + tier2Session.ID + `","tool_name":"symbolic_shell","input":{"command":"go_version"}}`)
	tier2Req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", tier2Body)
	tier2Rec := httptest.NewRecorder()
	server.handleToolExecute(tier2Rec, tier2Req)
	if tier2Rec.Code != http.StatusNotFound {
		t.Fatalf("expected symbolic_shell to be hidden for tier2 session (404), got %d: %s", tier2Rec.Code, tier2Rec.Body.String())
	}
}
