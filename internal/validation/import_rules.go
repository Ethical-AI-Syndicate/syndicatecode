package validation

import (
	"fmt"
	"slices"
	"strings"
)

type ImportViolation struct {
	Package    string
	ImportPath string
	Message    string
}

func (b BoundarySpec) ValidateImports(pkg string, imports []string) ([]ImportViolation, error) {
	boundary, ok := b.Packages[pkg]
	if !ok {
		return nil, fmt.Errorf("package %s is not defined in boundary spec", pkg)
	}

	violations := make([]ImportViolation, 0)
	for _, importPath := range imports {
		if !strings.HasPrefix(importPath, "internal/") && !strings.HasPrefix(importPath, "cmd/") {
			continue
		}

		if importPath == pkg {
			continue
		}

		if !slices.Contains(boundary.AllowedImports, importPath) {
			violations = append(violations, ImportViolation{
				Package:    pkg,
				ImportPath: importPath,
				Message:    fmt.Sprintf("package %s imports forbidden dependency %s", pkg, importPath),
			})
		}
	}

	return violations, nil
}
