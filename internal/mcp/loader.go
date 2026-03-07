package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type Plugin struct {
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	TrustLevel string                 `json:"trust_level"`
	Tools      []tools.ToolDefinition `json:"tools"`
}

type EventLogger interface {
	LogPluginEvent(ctx context.Context, pluginName, action string, payload map[string]interface{}) error
}

type Loader struct {
	registry *tools.Registry
	logger   EventLogger
}

func NewLoader(registry *tools.Registry, logger EventLogger) *Loader {
	return &Loader{registry: registry, logger: logger}
}

func (l *Loader) LoadFromFile(ctx context.Context, manifestPath string) (*Plugin, error) {
	if l == nil {
		return nil, fmt.Errorf("loader is nil")
	}
	if l.registry == nil {
		return nil, fmt.Errorf("registry is required")
	}

	validatedPath, err := validateManifestPath(manifestPath)
	if err != nil {
		return nil, err
	}

	body, err := os.ReadFile(validatedPath) // #nosec G304 -- path is normalized, extension-checked, and verified as an existing regular file
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest %s: %w", validatedPath, err)
	}

	var plugin Plugin
	if err := json.Unmarshal(body, &plugin); err != nil {
		return nil, fmt.Errorf("failed to parse manifest %s: %w", validatedPath, err)
	}

	if err := validatePlugin(plugin); err != nil {
		return nil, err
	}

	for _, def := range plugin.Tools {
		if err := validateToolForTrust(plugin.TrustLevel, def); err != nil {
			return nil, err
		}
		if err := l.registry.Register(def); err != nil {
			return nil, fmt.Errorf("failed to register plugin tool %s: %w", def.Name, err)
		}
	}

	if l.logger != nil {
		if err := l.logger.LogPluginEvent(ctx, plugin.Name, "loaded", map[string]interface{}{
			"manifest_path": validatedPath,
			"trust_level":   plugin.TrustLevel,
			"tool_count":    len(plugin.Tools),
		}); err != nil {
			return nil, fmt.Errorf("failed to log plugin event for %s: %w", plugin.Name, err)
		}
	}

	return &plugin, nil
}

func (l *Loader) LoadFromDir(ctx context.Context, manifestDir string) ([]*Plugin, error) {
	if manifestDir == "" {
		return nil, fmt.Errorf("manifest directory is required")
	}

	cleanDir := filepath.Clean(manifestDir)
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest directory %s: %w", cleanDir, err)
	}

	plugins := make([]*Plugin, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		manifestPath := filepath.Join(cleanDir, entry.Name())
		plugin, err := l.LoadFromFile(ctx, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load plugin manifest %s: %w", manifestPath, err)
		}
		plugins = append(plugins, plugin)
	}

	return plugins, nil
}

func validatePlugin(plugin Plugin) error {
	if plugin.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if plugin.Version == "" {
		return fmt.Errorf("plugin version is required")
	}
	if !isValidTrustLevel(plugin.TrustLevel) {
		return fmt.Errorf("invalid trust level: %s", plugin.TrustLevel)
	}
	if plugin.Tools == nil {
		return fmt.Errorf("plugin tools are required")
	}
	return nil
}

func isValidTrustLevel(trustLevel string) bool {
	switch trustLevel {
	case "tier0", "tier1", "tier2", "tier3":
		return true
	default:
		return false
	}
}

func validateToolForTrust(trustLevel string, def tools.ToolDefinition) error {
	if trustLevel != "tier0" {
		return nil
	}

	if def.SideEffect == tools.SideEffectExecute || def.SideEffect == tools.SideEffectNetwork || def.SideEffect == tools.SideEffectWrite {
		return fmt.Errorf("trust level %s cannot register tool %s with side effect %s", trustLevel, def.Name, def.SideEffect)
	}
	return nil
}

func validateManifestPath(manifestPath string) (string, error) {
	if manifestPath == "" {
		return "", fmt.Errorf("manifest path is required")
	}

	cleanPath := filepath.Clean(manifestPath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve manifest path %s: %w", manifestPath, err)
	}

	if !strings.EqualFold(filepath.Ext(absPath), ".json") {
		return "", fmt.Errorf("manifest must be a .json file: %s", absPath)
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat manifest %s: %w", absPath, err)
	}
	if fileInfo.IsDir() {
		return "", fmt.Errorf("manifest path must be a file: %s", absPath)
	}

	return absPath, nil
}
