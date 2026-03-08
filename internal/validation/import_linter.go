package validation

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type LintViolation struct {
	Package    string
	FilePath   string
	ImportPath string
	Message    string
}

func LintImportRules(root string, spec BoundarySpec) ([]LintViolation, error) {
	goFiles, err := findGoFiles(root)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	violations := make([]LintViolation, 0)

	for _, filePath := range goFiles {
		relPath, err := filepath.Rel(root, filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute relative path for %s: %w", filePath, err)
		}

		packagePath := filepath.ToSlash(filepath.Dir(relPath))
		if _, tracked := spec.Packages[packagePath]; !tracked {
			continue
		}

		file, err := parser.ParseFile(fset, filePath, nil, parser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("failed to parse imports in %s: %w", filePath, err)
		}

		imports := make([]string, 0, len(file.Imports))
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			internalPath := trimModulePrefix(path)
			imports = append(imports, internalPath)
		}

		packageViolations, err := spec.ValidateImports(packagePath, imports)
		if err != nil {
			return nil, err
		}

		for _, v := range packageViolations {
			violations = append(violations, LintViolation{
				Package:    v.Package,
				FilePath:   relPath,
				ImportPath: v.ImportPath,
				Message:    v.Message,
			})
		}
	}

	return violations, nil
}

func findGoFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk go files from %s: %w", root, err)
	}
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("failed to stat root %s: %w", root, err)
	}
	return files, nil
}

func trimModulePrefix(importPath string) string {
	prefix := "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/"
	if strings.HasPrefix(importPath, prefix) {
		return strings.TrimPrefix(importPath, prefix)
	}
	return importPath
}
