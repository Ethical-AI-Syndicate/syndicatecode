package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestLoadConfiguredPlugins_RegistersManifestToolsAndLogsEvent(t *testing.T) {
	t.Setenv(pluginManifestDirEnv, t.TempDir())
	pluginDir := os.Getenv(pluginManifestDirEnv)

	manifest := `{
	  "name": "controlplane-plugin",
	  "version": "1.0.0",
	  "trust_level": "tier1",
	  "identity": {"plugin_id": "controlplane.plugin", "publisher": "ai-syndicate"},
	  "approval_defaults": {"read": false, "write": true, "execute": true, "network": true},
	  "tools": [
	    {
	      "name": "controlplane_plugin_read",
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
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write plugin manifest: %v", err)
	}

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
	if err := loadConfiguredPlugins(context.Background(), registry, eventStore); err != nil {
		t.Fatalf("expected plugin loading to succeed: %v", err)
	}

	if _, ok := registry.Get("controlplane_plugin_read"); !ok {
		t.Fatal("expected plugin tool to be registered")
	}

	events, err := eventStore.QueryAll(context.Background())
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one plugin audit event, got %d", len(events))
	}
	if events[0].EventType != "plugin.loaded" {
		t.Fatalf("unexpected plugin event type: %s", events[0].EventType)
	}
}

func TestLoadConfiguredPlugins_NoEnvConfiguredSkipsLoading(t *testing.T) {
	t.Setenv(pluginManifestDirEnv, "")

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
	if err := loadConfiguredPlugins(context.Background(), registry, eventStore); err != nil {
		t.Fatalf("expected no-op plugin loading, got: %v", err)
	}
}
