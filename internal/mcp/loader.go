package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type Plugin struct {
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	TrustLevel string                 `json:"trust_level"`
	Identity   PluginIdentity         `json:"identity"`
	Defaults   ApprovalDefaults       `json:"approval_defaults"`
	Tools      []tools.ToolDefinition `json:"tools"`
}

type PluginIdentity struct {
	PluginID  string `json:"plugin_id"`
	Publisher string `json:"publisher"`
}

type ApprovalDefaults struct {
	Read    bool `json:"read"`
	Write   bool `json:"write"`
	Execute bool `json:"execute"`
	Network bool `json:"network"`
}

type ManifestTool struct {
	tools.ToolDefinition
	DataAccessClass string             `json:"data_access_class"`
	NetworkScope    string             `json:"network_scope"`
	MCP             *tools.MCPMetadata `json:"mcp,omitempty"`
}

type rawPluginManifest struct {
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	TrustLevel string           `json:"trust_level"`
	Identity   PluginIdentity   `json:"identity"`
	Defaults   ApprovalDefaults `json:"approval_defaults"`
	Tools      []ManifestTool   `json:"tools"`
}

type EventLogger interface {
	LogPluginEvent(ctx context.Context, pluginName, action string, payload map[string]interface{}) error
}

type Loader struct {
	registry  *tools.Registry
	logger    EventLogger
	inventory *ExtensionInventory
}

func NewLoader(registry *tools.Registry, logger EventLogger) *Loader {
	return &Loader{registry: registry, logger: logger, inventory: NewExtensionInventory()}
}

func (l *Loader) Inventory() []Plugin {
	if l == nil || l.inventory == nil {
		return nil
	}
	return l.inventory.List()
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

	var raw rawPluginManifest
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse manifest %s: %w", validatedPath, err)
	}

	plugin, err := toPlugin(raw)
	if err != nil {
		return nil, err
	}

	if err := validatePlugin(*plugin); err != nil {
		return nil, err
	}

	for _, def := range plugin.Tools {
		def.Source = tools.ToolSourcePlugin
		def.TrustLevel = plugin.TrustLevel
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
			"plugin_id":     plugin.Identity.PluginID,
			"publisher":     plugin.Identity.Publisher,
			"tool_count":    len(plugin.Tools),
		}); err != nil {
			return nil, fmt.Errorf("failed to log plugin event for %s: %w", plugin.Name, err)
		}
	}

	l.inventory.Upsert(*plugin)

	return plugin, nil
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
	if plugin.Identity.PluginID == "" {
		return fmt.Errorf("plugin identity.plugin_id is required")
	}
	if plugin.Identity.Publisher == "" {
		return fmt.Errorf("plugin identity.publisher is required")
	}
	if plugin.Tools == nil {
		return fmt.Errorf("plugin tools are required")
	}
	return nil
}

func toPlugin(raw rawPluginManifest) (*Plugin, error) {
	defs := make([]tools.ToolDefinition, 0, len(raw.Tools))
	for _, toolDef := range raw.Tools {
		if toolDef.DataAccessClass == "" {
			return nil, fmt.Errorf("tool %s data_access_class is required", toolDef.Name)
		}
		if !isValidDataAccessClass(toolDef.DataAccessClass) {
			return nil, fmt.Errorf("tool %s has invalid data_access_class %s", toolDef.Name, toolDef.DataAccessClass)
		}
		if toolDef.NetworkScope == "" {
			return nil, fmt.Errorf("tool %s network_scope is required", toolDef.Name)
		}
		if !isValidNetworkScope(toolDef.NetworkScope) {
			return nil, fmt.Errorf("tool %s has invalid network_scope %s", toolDef.Name, toolDef.NetworkScope)
		}

		tool := toolDef.ToolDefinition
		tool.Security = tools.SecurityMetadata{
			NetworkAccess:   toolDef.NetworkScope != "none",
			FilesystemScope: toolDef.DataAccessClass,
		}
		if toolDef.MCP != nil {
			if err := validateMCPMetadata(*toolDef.MCP, toolDef.NetworkScope, toolDef.Name); err != nil {
				return nil, err
			}
			metadataCopy := *toolDef.MCP
			metadataCopy.AllowedDestinations = append([]string(nil), toolDef.MCP.AllowedDestinations...)
			tool.MCP = &metadataCopy
		}
		if !tool.ApprovalRequired {
			tool.ApprovalRequired = approvalDefaultForSideEffect(raw.Defaults, tool.SideEffect)
		}

		defs = append(defs, tool)
	}

	return &Plugin{
		Name:       raw.Name,
		Version:    raw.Version,
		TrustLevel: raw.TrustLevel,
		Identity:   raw.Identity,
		Defaults:   raw.Defaults,
		Tools:      defs,
	}, nil
}

func approvalDefaultForSideEffect(defaults ApprovalDefaults, sideEffect tools.SideEffect) bool {
	switch sideEffect {
	case tools.SideEffectRead:
		return defaults.Read
	case tools.SideEffectWrite:
		return defaults.Write
	case tools.SideEffectExecute:
		return defaults.Execute
	case tools.SideEffectNetwork:
		return defaults.Network
	default:
		return false
	}
}

func isValidDataAccessClass(value string) bool {
	switch value {
	case "none", "metadata", "workspace_read", "workspace_write", "secrets":
		return true
	default:
		return false
	}
}

func isValidNetworkScope(value string) bool {
	switch value {
	case "none", "egress_limited", "egress_open":
		return true
	default:
		return false
	}
}

func validateMCPMetadata(metadata tools.MCPMetadata, networkScope, toolName string) error {
	if metadata.ServerID == "" {
		return fmt.Errorf("tool %s mcp.server_id is required", toolName)
	}
	if metadata.Transport != "local" && metadata.Transport != "remote" {
		return fmt.Errorf("tool %s has invalid mcp.transport %s", toolName, metadata.Transport)
	}
	if metadata.Transport == "remote" {
		if networkScope == "none" {
			return fmt.Errorf("tool %s remote mcp transport requires network access", toolName)
		}
		if len(metadata.AllowedDestinations) == 0 {
			return fmt.Errorf("tool %s remote mcp transport requires allowed_destinations", toolName)
		}
	}
	for _, destination := range metadata.AllowedDestinations {
		if strings.TrimSpace(destination) == "" {
			return fmt.Errorf("tool %s mcp.allowed_destinations cannot contain empty values", toolName)
		}
	}
	return nil
}

type ExtensionInventory struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewExtensionInventory() *ExtensionInventory {
	return &ExtensionInventory{plugins: make(map[string]Plugin)}
}

func (i *ExtensionInventory) Upsert(plugin Plugin) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.plugins[plugin.Identity.PluginID] = plugin
}

func (i *ExtensionInventory) List() []Plugin {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make([]Plugin, 0, len(i.plugins))
	for _, plugin := range i.plugins {
		result = append(result, plugin)
	}
	return result
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
