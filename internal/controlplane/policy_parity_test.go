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

func TestHandleToolExecute_CoreAndPluginMetadataScopeParity(t *testing.T) {
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
			Name:             "core_meta",
			Version:          "1",
			Source:           tools.ToolSourceCore,
			SideEffect:       tools.SideEffectRead,
			ApprovalRequired: false,
			InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}},
			OutputSchema:     map[string]tools.FieldSchema{"ok": {Type: "boolean"}},
			Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
			Security:         tools.SecurityMetadata{FilesystemScope: "metadata"},
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
	}
	for _, def := range defs {
		if err := registry.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.Name, err)
		}
	}

	executor := tools.NewExecutor(registry, nil)
	executor.RegisterHandler("core_meta", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"ok": true}, nil
	})
	executor.RegisterHandler("plugin_meta", func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		_ = ctx
		_ = input
		return map[string]interface{}{"ok": true}, nil
	})

	server := &Server{
		sessionMgr:   sessionMgr,
		approvalMgr:  NewApprovalManager(10 * time.Minute),
		toolRegistry: registry,
		toolExecutor: executor,
		eventStore:   eventStore,
		httpServer:   &http.Server{},
	}

	for _, toolName := range []string{"core_meta", "plugin_meta"} {
		body := bytes.NewBufferString(`{"session_id":"` + created.ID + `","tool_name":"` + toolName + `","input":{"path":"README.md"}}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tools/execute", body)
		rec := httptest.NewRecorder()

		server.handleToolExecute(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s metadata path access, got %d: %s", toolName, rec.Code, rec.Body.String())
		}
	}
}
