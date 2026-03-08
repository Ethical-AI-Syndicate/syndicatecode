package controlplane

import (
	"bytes"
	"context"
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

func TestHandleToolExecute_PluginInstalledButHiddenForRestrictedSession(t *testing.T) {
	server, registry, sessionID := newPluginExposureTestServer(t, "tier3")

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_read","input":{"path":"README.md"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected hidden plugin tool to return 404, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, ok := registry.Get("plugin_read"); !ok {
		t.Fatal("expected plugin tool to remain installed in registry")
	}
}

func TestHandleToolExecute_PluginVisibleForTier1Session(t *testing.T) {
	server, _, sessionID := newPluginExposureTestServer(t, "tier1")

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_read","input":{"path":"README.md"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected plugin tool to execute for tier1 session, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleToolExecute_PluginHiddenWhenPluginTrustLowerThanSessionTrust(t *testing.T) {
	server, _, sessionID := newPluginExposureTestServer(t, "tier2")

	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_read","input":{"path":"README.md"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected plugin tool to be hidden when plugin trust is lower than session trust, got %d: %s", rec.Code, rec.Body.String())
	}
}

func newPluginExposureTestServer(t *testing.T, trustTier string) (*Server, *tools.Registry, string) {
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
	  "name": "exposure-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {
	    "plugin_id": "exposure-plugin",
	    "publisher": "test"
	  },
	  "approval_defaults": {
	    "read": false,
	    "write": true,
	    "execute": true,
	    "network": true
	  },
	  "tools": [
	    {
	      "name": "plugin_read",
	      "version": "1",
	      "side_effect": "read",
	      "approval_required": false,
	      "data_access_class": "workspace_read",
	      "network_scope": "none",
	      "input_schema": {"path": {"type": "string"}},
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
	executor.RegisterHandler("plugin_read", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"content": "ok"}, nil
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), trustTier)
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
	}, registry, created.ID
}
