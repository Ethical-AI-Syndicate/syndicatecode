package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type stubLogger struct {
	events []string
}

func (s *stubLogger) LogPluginEvent(ctx context.Context, pluginName, action string, payload map[string]interface{}) error {
	_ = ctx
	_ = payload
	s.events = append(s.events, pluginName+":"+action)
	return nil
}

func TestLoader_LoadManifestRegistersTools(t *testing.T) {
	registry := tools.NewRegistry()
	logger := &stubLogger{}
	loader := NewLoader(registry, logger)

	manifest := `{
	  "name": "example-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "example.plugin", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
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

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "plugin.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	plugin, err := loader.LoadFromFile(context.Background(), manifestPath)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if plugin.Name != "example-plugin" {
		t.Fatalf("unexpected plugin name: %s", plugin.Name)
	}
	if _, ok := registry.Get("plugin_read"); !ok {
		t.Fatal("expected plugin tool to be registered")
	}
	if len(logger.events) != 1 {
		t.Fatalf("expected one plugin event, got %d", len(logger.events))
	}
	if plugin.Identity.PluginID != "example.plugin" {
		t.Fatalf("unexpected plugin identity id: %s", plugin.Identity.PluginID)
	}
}

func TestLoader_RejectsInvalidTrustLevel(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	manifest := `{"name":"bad","version":"1.0.0","trust_level":"tier9","identity":{"plugin_id":"bad","publisher":"ai"},"approval_defaults":{"read":false,"write":true,"execute":true,"network":true},"tools":[]}`
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err == nil {
		t.Fatal("expected invalid trust level to fail")
	}
}

func TestLoader_RejectsTier0DangerousTool(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	manifest := `{
	  "name": "bad-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier0",
	  "identity": {"plugin_id": "bad.plugin", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
	  "tools": [
	    {
	      "name": "plugin_exec",
	      "version": "1",
	      "side_effect": "execute",
	      "approval_required": true,
	      "data_access_class": "workspace_read",
	      "network_scope": "none",
	      "input_schema": {},
	      "output_schema": {},
	      "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	    }
	  ]
	}`

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "bad-plugin.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err == nil {
		t.Fatal("expected tier0 dangerous tool to be rejected")
	}
}

func TestLoader_RejectsNonJSONManifestPath(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "plugin.txt")
	if err := os.WriteFile(manifestPath, []byte(`{"name":"bad"}`), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err == nil {
		t.Fatal("expected non-json manifest path to be rejected")
	}
}

func TestLoader_LoadFromDirectoryRegistersJSONManifests(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	tmpDir := t.TempDir()
	manifestA := `{
	  "name": "plugin-a",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "plugin.a", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
	  "tools": [
	    {
	      "name": "plugin_a_read",
	      "version": "1",
	      "side_effect": "read",
	      "approval_required": false,
	      "data_access_class": "workspace_read",
	      "network_scope": "none",
	      "input_schema": {},
	      "output_schema": {},
	      "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	    }
	  ]
	}`
	manifestB := `{
	  "name": "plugin-b",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "plugin.b", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
	  "tools": [
	    {
	      "name": "plugin_b_read",
	      "version": "1",
	      "side_effect": "read",
	      "approval_required": false,
	      "data_access_class": "workspace_read",
	      "network_scope": "none",
	      "input_schema": {},
	      "output_schema": {},
	      "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	    }
	  ]
	}`

	if err := os.WriteFile(filepath.Join(tmpDir, "a.json"), []byte(manifestA), 0644); err != nil {
		t.Fatalf("failed to write plugin A manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.json"), []byte(manifestB), 0644); err != nil {
		t.Fatalf("failed to write plugin B manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatalf("failed to write non-manifest file: %v", err)
	}

	plugins, err := loader.LoadFromDir(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("failed to load plugins from directory: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
	if _, ok := registry.Get("plugin_a_read"); !ok {
		t.Fatal("expected plugin_a_read tool to be registered")
	}
	if _, ok := registry.Get("plugin_b_read"); !ok {
		t.Fatal("expected plugin_b_read tool to be registered")
	}
}

func TestLoader_RejectsMissingIdentityAndApprovalDefaults(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	manifest := `{
	  "name": "missing-schema-fields",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "tools": [{
	    "name": "plugin_read",
	    "version": "1",
	    "side_effect": "read",
	    "data_access_class": "workspace_read",
	    "network_scope": "none",
	    "input_schema": {},
	    "output_schema": {},
	    "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	  }]
	}`

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "missing-fields.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err == nil {
		t.Fatal("expected manifest missing identity/approval_defaults to fail")
	}
}

func TestLoader_RejectsInvalidToolScopeFields(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	manifest := `{
	  "name": "bad-scopes",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "bad.scopes", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
	  "tools": [{
	    "name": "plugin_read",
	    "version": "1",
	    "side_effect": "read",
	    "data_access_class": "unknown",
	    "network_scope": "wild",
	    "input_schema": {},
	    "output_schema": {},
	    "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	  }]
	}`

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "invalid-scopes.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err == nil {
		t.Fatal("expected invalid data_access_class/network_scope to fail")
	}
}

func TestLoader_PersistsValidatedManifestInInventory(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	manifest := `{
	  "name": "inventory-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "inventory.plugin", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": true, "write": true, "execute": true, "network": true},
	  "tools": [{
	    "name": "plugin_read",
	    "version": "1",
	    "side_effect": "read",
	    "data_access_class": "workspace_read",
	    "network_scope": "none",
	    "input_schema": {},
	    "output_schema": {},
	    "limits": {"timeout_seconds": 10, "max_output_bytes": 1024}
	  }]
	}`

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "inventory.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	if _, err := loader.LoadFromFile(context.Background(), manifestPath); err != nil {
		t.Fatalf("expected manifest to load: %v", err)
	}

	inventory := loader.Inventory()
	if len(inventory) != 1 {
		t.Fatalf("expected one inventory entry, got %d", len(inventory))
	}
	if inventory[0].Identity.PluginID != "inventory.plugin" {
		t.Fatalf("unexpected inventory plugin id: %s", inventory[0].Identity.PluginID)
	}
}

func TestLoader_LoadFromDirectoryRejectsSubdirectoryEntries(t *testing.T) {
	registry := tools.NewRegistry()
	loader := NewLoader(registry, nil)

	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "nested"), 0755); err != nil {
		t.Fatalf("failed to create nested directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "nested", "plugin.json"), []byte(`{}`), 0644); err != nil {
		t.Fatalf("failed to write nested plugin file: %v", err)
	}

	plugins, err := loader.LoadFromDir(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("expected nested directories to be ignored, got error: %v", err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected no plugins from nested directories, got %d", len(plugins))
	}
}
