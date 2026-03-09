package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/mcp"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestHandleToolExecute_RemoteMCPDestinationDeniedWhenNotAllowlisted(t *testing.T) {
	server, _, sessionID := newMCPTransportTestServer(t)

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"remote_mcp_fetch","input":{"destination":"https://blocked.example.com","query":"status"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected remote MCP destination denial (403), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleToolExecute_RemoteMCPCallWritesAuditMetadata(t *testing.T) {
	server, eventStore, sessionID := newMCPTransportTestServer(t)

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"remote_mcp_fetch","input":{"destination":"https://allowed.example.com","query":"status"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected pending approval for side-effecting remote MCP tool, got %d: %s", rec.Code, rec.Body.String())
	}

	var approval Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &approval); err != nil {
		t.Fatalf("failed to decode approval response: %v", err)
	}

	decisionBody := bytes.NewBufferString(`{"decision":"approve"}`)
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.ID, decisionBody)
	decisionRec := httptest.NewRecorder()
	server.handleApprovalByID(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("expected approval execution success, got %d: %s", decisionRec.Code, decisionRec.Body.String())
	}

	events, err := eventStore.QueryAll(context.Background())
	if err != nil {
		t.Fatalf("failed to query event store: %v", err)
	}

	var auditEvent *audit.Event
	for idx := range events {
		if events[idx].EventType == "mcp.call" {
			auditEvent = &events[idx]
			break
		}
	}
	if auditEvent == nil {
		t.Fatalf("expected mcp.call audit event, found %d events", len(events))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(auditEvent.Payload, &payload); err != nil {
		t.Fatalf("failed to decode mcp.call payload: %v", err)
	}

	if payload["transport"] != "remote" {
		t.Fatalf("expected transport=remote, got %v", payload["transport"])
	}
	if payload["server_id"] != "inventory.remote" {
		t.Fatalf("expected server_id=inventory.remote, got %v", payload["server_id"])
	}
	if payload["destination"] != "https://allowed.example.com" {
		t.Fatalf("expected destination metadata, got %v", payload["destination"])
	}
	if _, ok := payload["request"]; !ok {
		t.Fatal("expected request metadata in mcp.call payload")
	}
	if _, ok := payload["response"]; !ok {
		t.Fatal("expected response metadata in mcp.call payload")
	}
}

func TestHandleSessionByID_FilteredEventsIncludeMCPRoutingTrace(t *testing.T) {
	server, _, sessionID := newMCPTransportTestServer(t)

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"remote_mcp_fetch","input":{"destination":"https://allowed.example.com","query":"status"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected pending approval for side-effecting remote MCP tool, got %d: %s", rec.Code, rec.Body.String())
	}

	var approval Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &approval); err != nil {
		t.Fatalf("failed to decode approval response: %v", err)
	}

	decisionBody := bytes.NewBufferString(`{"decision":"approve"}`)
	decisionReq := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.ID, decisionBody)
	decisionRec := httptest.NewRecorder()
	server.handleApprovalByID(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("expected approval execution success, got %d: %s", decisionRec.Code, decisionRec.Body.String())
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/events?event_type=mcp.call", nil)
	eventsRec := httptest.NewRecorder()
	server.handleSessionByID(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for filtered session events, got %d: %s", eventsRec.Code, eventsRec.Body.String())
	}

	var events []audit.Event
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &events); err != nil {
		t.Fatalf("failed to decode filtered replay events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one mcp.call event, got %d", len(events))
	}
	if events[0].EventType != "mcp.call" {
		t.Fatalf("expected mcp.call event type, got %s", events[0].EventType)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("failed to decode mcp.call payload from replay endpoint: %v", err)
	}
	if payload["transport"] != "remote" {
		t.Fatalf("expected transport=remote, got %v", payload["transport"])
	}
	if payload["server_id"] != "inventory.remote" {
		t.Fatalf("expected server_id=inventory.remote, got %v", payload["server_id"])
	}
	if payload["destination"] != "https://allowed.example.com" {
		t.Fatalf("expected destination metadata, got %v", payload["destination"])
	}
}

func newMCPTransportTestServer(t *testing.T) (*Server, *audit.EventStore, string) {
	t.Helper()

	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	registry := tools.NewRegistry()
	pluginLoader := mcp.NewLoader(registry, nil)

	manifest := `{
	  "name": "inventory-mcp-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {
	    "plugin_id": "inventory.plugin",
	    "publisher": "test"
	  },
	  "approval_defaults": {
	    "read": false,
	    "write": true,
	    "execute": true,
	    "network": false
	  },
	  "tools": [
	    {
	      "name": "remote_mcp_fetch",
	      "version": "1",
	      "side_effect": "network",
	      "approval_required": false,
	      "data_access_class": "metadata",
	      "network_scope": "egress_limited",
	      "mcp": {
	        "server_id": "inventory.remote",
	        "transport": "remote",
	        "allowed_destinations": ["https://allowed.example.com"]
	      },
	      "input_schema": {
	        "destination": {"type": "string", "required": true},
	        "query": {"type": "string", "required": true}
	      },
	      "output_schema": {"content": {"type": "string"}},
	      "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	    }
	  ]
	}`
	manifestPath := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write plugin manifest: %v", err)
	}
	if _, err := pluginLoader.LoadFromFile(context.Background(), manifestPath); err != nil {
		t.Fatalf("failed to load plugin manifest: %v", err)
	}

	executor := tools.NewExecutor(registry, nil)
	executor.RegisterHandler("remote_mcp_fetch", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"content": "ok"}, nil
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	return &Server{
		sessionMgr:   sessionMgr,
		toolRegistry: registry,
		toolExecutor: executor,
		eventStore:   eventStore,
		httpServer:   &http.Server{},
		approvalMgr:  NewApprovalManager(15 * time.Minute),
	}, eventStore, created.ID
}
