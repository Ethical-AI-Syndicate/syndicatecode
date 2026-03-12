package validation

import (
	"fmt"
	"sort"
)

type PackageBoundary struct {
	Owner            string
	Responsibilities []string
	AllowedImports   []string
}

type BoundarySpec struct {
	Packages map[string]PackageBoundary
}

func DefaultPackageBoundarySpec() BoundarySpec {
	return BoundarySpec{
		Packages: map[string]PackageBoundary{
			"cmd/cli": {
				Owner:            "platform",
				Responsibilities: []string{"bootstrap interactive CLI"},
				AllowedImports:   []string{"pkg/tui"},
			},
			"cmd/server": {
				Owner:            "platform",
				Responsibilities: []string{"bootstrap control-plane server"},
				AllowedImports:   []string{"internal/controlplane"},
			},
			"internal/controlplane": {
				Owner:            "controlplane",
				Responsibilities: []string{"request orchestration", "session and turn coordination", "policy surface integration"},
				AllowedImports:   []string{"internal/agent", "internal/audit", "internal/context", "internal/mcp", "internal/models", "internal/models/anthropic", "internal/models/openai", "internal/patch", "internal/policy", "internal/requestmeta", "internal/sandbox", "internal/secrets", "internal/session", "internal/state", "internal/tools", "internal/validation"},
			},
			"internal/session": {
				Owner:            "state",
				Responsibilities: []string{"session lifecycle"},
				AllowedImports:   []string{"internal/audit", "internal/requestmeta", "internal/state"},
			},
			"internal/context": {
				Owner:            "ai-systems",
				Responsibilities: []string{"turn lifecycle", "context assembly", "token budgeting", "retrieval profile classification"},
				AllowedImports:   []string{"internal/audit", "internal/requestmeta", "internal/secrets", "internal/session", "internal/state"},
			},
			"internal/agent": {
				Owner:            "ai-systems",
				Responsibilities: []string{"react loop", "model/tool orchestration", "reliability limits"},
				AllowedImports:   []string{"internal/models", "internal/tools", "internal/trust"},
			},
			"internal/state": {
				Owner:            "state",
				Responsibilities: []string{"canonical lifecycle enums", "state transition validation"},
				AllowedImports:   []string{},
			},
			"internal/audit": {
				Owner:            "platform",
				Responsibilities: []string{"event persistence", "event queries"},
				AllowedImports:   []string{},
			},
			"internal/git": {
				Owner:            "runtime",
				Responsibilities: []string{"task-scoped git safety and provenance"},
				AllowedImports:   []string{},
			},
			"internal/models": {
				Owner:            "ai-systems",
				Responsibilities: []string{"provider-agnostic model abstraction", "streaming event contracts"},
				AllowedImports:   []string{},
			},
			"internal/models/anthropic": {
				Owner:            "ai-systems",
				Responsibilities: []string{"Anthropic SDK streaming provider implementation"},
				AllowedImports:   []string{"internal/models"},
			},
			"internal/models/openai": {
				Owner:            "ai-systems",
				Responsibilities: []string{"OpenAI SDK streaming provider implementation"},
				AllowedImports:   []string{"internal/models"},
			},
			"internal/mcp": {
				Owner:            "integrations",
				Responsibilities: []string{"MCP plugin loading and registry integration"},
				AllowedImports:   []string{"internal/tools"},
			},
			"internal/patch": {
				Owner:            "runtime",
				Responsibilities: []string{"patch parsing and repository-safe file edits"},
				AllowedImports:   []string{},
			},
			"internal/policy": {
				Owner:            "governance",
				Responsibilities: []string{"policy contracts and evaluation surface"},
				AllowedImports:   []string{},
			},
			"internal/trust": {
				Owner:            "governance",
				Responsibilities: []string{"trust-tier policy resolution", "side-effect/approval rules"},
				AllowedImports:   []string{"internal/tools"},
			},
			"internal/secrets": {
				Owner:            "security",
				Responsibilities: []string{"secret detection, classification, and redaction policy"},
				AllowedImports:   []string{},
			},
			"internal/tools": {
				Owner:            "runtime",
				Responsibilities: []string{"tool capability registry", "execution preflight checks"},
				AllowedImports:   []string{"internal/git", "internal/patch"},
			},
			"internal/sandbox": {
				Owner:            "runtime",
				Responsibilities: []string{"isolation-level runners", "resource limits", "execution controls"},
				AllowedImports:   []string{"internal/tools", "internal/trust"},
			},
			"internal/validation": {
				Owner:            "architecture",
				Responsibilities: []string{"architecture and boundary validation"},
				AllowedImports:   []string{},
			},
			"internal/requestmeta": {
				Owner:            "platform",
				Responsibilities: []string{"request-scoped actor and role metadata"},
				AllowedImports:   []string{},
			},
			"pkg/api": {
				Owner:            "platform",
				Responsibilities: []string{"stable API types and contract parsing"},
				AllowedImports:   []string{},
			},
			"pkg/tui": {
				Owner:            "platform",
				Responsibilities: []string{"terminal UI rendering and API client interactions"},
				AllowedImports:   []string{},
			},
		},
	}
}

func (b BoundarySpec) ValidateCoverage(existingPackages []string) error {
	missing := make([]string, 0)
	for _, pkg := range existingPackages {
		if _, ok := b.Packages[pkg]; !ok {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("missing package boundaries for: %v", missing)
}
