package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/mcp"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

const pluginManifestDirEnv = "SYNDICATE_PLUGIN_DIR"

type pluginAuditLogger struct {
	store *audit.EventStore
}

func (l *pluginAuditLogger) LogPluginEvent(ctx context.Context, pluginName, action string, payload map[string]interface{}) error {
	if l == nil || l.store == nil {
		return nil
	}

	payloadCopy := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		payloadCopy[key] = value
	}
	payloadCopy["plugin_name"] = pluginName

	encodedPayload, err := json.Marshal(payloadCopy)
	if err != nil {
		return fmt.Errorf("failed to encode plugin event payload: %w", err)
	}

	if err := l.store.Append(ctx, audit.Event{
		ID:        uuid.New().String(),
		SessionID: "system",
		Timestamp: time.Now().UTC(),
		EventType: "plugin." + action,
		Actor:     "controlplane",
		Payload:   encodedPayload,
	}); err != nil {
		return fmt.Errorf("failed to append plugin event: %w", err)
	}

	return nil
}

func loadConfiguredPlugins(ctx context.Context, registry *tools.Registry, eventStore *audit.EventStore) error {
	if registry == nil {
		return fmt.Errorf("registry is required")
	}

	pluginDir := strings.TrimSpace(os.Getenv(pluginManifestDirEnv))
	if pluginDir == "" {
		return nil
	}

	loader := mcp.NewLoader(registry, &pluginAuditLogger{store: eventStore})
	if _, err := loader.LoadFromDir(ctx, pluginDir); err != nil {
		return fmt.Errorf("failed to load configured plugins from %s: %w", pluginDir, err)
	}

	return nil
}
