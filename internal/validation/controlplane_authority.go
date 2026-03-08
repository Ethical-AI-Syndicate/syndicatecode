package validation

import (
	"fmt"
	"os"
	"path/filepath"
)

func ValidateControlPlaneAuthority(root string) ([]LintViolation, error) {
	moduleRoot, err := findModuleRoot(root)
	if err != nil {
		return nil, err
	}

	spec := DefaultPackageBoundarySpec()
	violations, err := LintImportRules(moduleRoot, spec)
	if err != nil {
		return nil, err
	}

	forbiddenBypass := map[string]bool{
		"internal/tools":   true,
		"internal/sandbox": true,
	}

	result := make([]LintViolation, 0, len(violations))
	for _, v := range violations {
		if !forbiddenBypass[v.ImportPath] {
			continue
		}
		if v.Package == "internal/controlplane" {
			continue
		}
		result = append(result, v)
	}

	return result, nil
}

func findModuleRoot(start string) (string, error) {
	current := filepath.Clean(start)
	for {
		candidate := filepath.Join(current, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return current, nil
		}

		next := filepath.Dir(current)
		if next == current {
			if _, err := os.Stat(start); err != nil {
				return "", fmt.Errorf("unable to use start path %s: %w", start, err)
			}
			return filepath.Clean(start), nil
		}
		current = next
	}
}
