package controlplane

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestHandleToolExecute_PluginSideEffectsRequireApproval(t *testing.T) {
	server, sessionID := newPluginPolicyTestServer(t)
	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_write","input":{"path":"a.txt","content":"x"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for side-effecting plugin tool, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleToolExecute_PluginMetadataScopeRejectsPathAccess(t *testing.T) {
	server, sessionID := newPluginPolicyTestServer(t)
	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_meta","input":{"path":"README.md"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for metadata scope path access, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleToolExecute_PluginRejectsImplicitFullContext(t *testing.T) {
	server, sessionID := newPluginPolicyTestServer(t)
	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_read","input":{"full_context":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for implicit full-context request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleToolExecute_PluginWorkspaceReadScopeAllowsPath(t *testing.T) {
	server, sessionID := newPluginPolicyTestServer(t)
	body := bytes.NewBufferString(`{"session_id":"` + sessionID + `","tool_name":"plugin_read","input":{"path":"README.md"}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
	rec := httptest.NewRecorder()

	server.handleToolExecute(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for workspace_read path access, got %d: %s", rec.Code, rec.Body.String())
	}
}

func newPluginPolicyTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	registry := tools.NewRegistry()
	defs := []tools.ToolDefinition{
		{
			Name:             "plugin_write",
			Version:          "1",
			Source:           tools.ToolSourcePlugin,
			TrustLevel:       "tier1",
			SideEffect:       tools.SideEffectWrite,
			ApprovalRequired: false,
			InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}},
			OutputSchema:     map[string]tools.FieldSchema{"ok": {Type: "boolean"}},
			Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
			Security:         tools.SecurityMetadata{FilesystemScope: "workspace_write"},
		},
		{
			Name:             "plugin_meta",
			Version:          "1",
			Source:           tools.ToolSourcePlugin,
			TrustLevel:       "tier1",
			SideEffect:       tools.SideEffectRead,
			ApprovalRequired: false,
			InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}},
			OutputSchema:     map[string]tools.FieldSchema{"ok": {Type: "boolean"}},
			Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
			Security:         tools.SecurityMetadata{FilesystemScope: "metadata"},
		},
		{
			Name:             "plugin_read",
			Version:          "1",
			Source:           tools.ToolSourcePlugin,
			TrustLevel:       "tier1",
			SideEffect:       tools.SideEffectRead,
			ApprovalRequired: false,
			InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}},
			OutputSchema:     map[string]tools.FieldSchema{"ok": {Type: "boolean"}},
			Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
			Security:         tools.SecurityMetadata{FilesystemScope: "workspace_read"},
		},
	}

	for _, def := range defs {
		if err := registry.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.Name, err)
		}
	}

	executor := tools.NewExecutor(registry, nil)
	executor.RegisterHandler("plugin_write", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"ok": true}, nil
	})
	executor.RegisterHandler("plugin_meta", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"ok": true}, nil
	})
	executor.RegisterHandler("plugin_read", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"ok": true}, nil
	})

	return &Server{
		sessionMgr:   sessionMgr,
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
		eventStore:   eventStore,
		httpServer:   &http.Server{},
	}, created.ID
}
