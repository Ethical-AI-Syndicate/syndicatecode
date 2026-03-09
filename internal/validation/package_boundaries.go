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
				Responsibilities: []string{"bootstrap terminal ui client"},
				AllowedImports:   []string{},
			},
			"cmd/server": {
				Owner:            "platform",
				Responsibilities: []string{"bootstrap control-plane server"},
				AllowedImports:   []string{"internal/controlplane"},
			},
			"internal/controlplane": {
				Owner:            "controlplane",
				Responsibilities: []string{"request orchestration", "session and turn coordination", "policy surface integration"},
				AllowedImports:   []string{"internal/session", "internal/context", "internal/audit", "internal/tools", "internal/sandbox", "internal/mcp", "internal/patch", "internal/secrets", "internal/validation"},
			},
			"internal/session": {
				Owner:            "state",
				Responsibilities: []string{"session lifecycle"},
				AllowedImports:   []string{"internal/audit"},
			},
			"internal/context": {
				Owner:            "ai-systems",
				Responsibilities: []string{"turn lifecycle", "context assembly", "token budgeting", "retrieval profile classification"},
				AllowedImports:   []string{"internal/audit", "internal/session", "internal/secrets"},
			},
			"internal/audit": {
				Owner:            "platform",
				Responsibilities: []string{"event persistence", "event queries"},
				AllowedImports:   []string{},
			},
			"internal/git": {
				Owner:            "platform",
				Responsibilities: []string{"git integration and repository state inspection"},
				AllowedImports:   []string{},
			},
			"internal/mcp": {
				Owner:            "runtime",
				Responsibilities: []string{"mcp manifest policy and transport controls"},
				AllowedImports:   []string{"internal/tools"},
			},
			"internal/patch": {
				Owner:            "runtime",
				Responsibilities: []string{"patch parsing, validation, and application"},
				AllowedImports:   []string{},
			},
			"internal/policy": {
				Owner:            "policy",
				Responsibilities: []string{"provider policy bundle and routing constraints"},
				AllowedImports:   []string{},
			},
			"internal/secrets": {
				Owner:            "security",
				Responsibilities: []string{"secret detection, classification, and redaction policy"},
				AllowedImports:   []string{},
			},
			"internal/tools": {
				Owner:            "runtime",
				Responsibilities: []string{"tool capability registry", "execution preflight checks"},
				AllowedImports:   []string{"internal/patch"},
			},
			"internal/sandbox": {
				Owner:            "runtime",
				Responsibilities: []string{"isolation-level runners", "resource limits", "execution controls"},
				AllowedImports:   []string{"internal/tools"},
			},
			"internal/validation": {
				Owner:            "architecture",
				Responsibilities: []string{"architecture and boundary validation"},
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
