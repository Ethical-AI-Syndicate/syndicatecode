package patch

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Result struct {
	ModifiedFiles []string
}

type Engine struct {
	repoRoot string
}

func NewEngine(repoRoot string) *Engine {
	return &Engine{repoRoot: repoRoot}
}

func (e *Engine) Apply(patchText string) (*Result, error) {
	lines := strings.Split(strings.ReplaceAll(patchText, "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" || strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return nil, fmt.Errorf("invalid patch envelope")
	}

	result := &Result{ModifiedFiles: make([]string, 0)}
	for i := 1; i < len(lines)-1; {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			next, err := e.applyAdd(path, lines, i+1)
			if err != nil {
				return nil, err
			}
			result.ModifiedFiles = append(result.ModifiedFiles, path)
			i = next
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			next, err := e.applyUpdate(path, lines, i+1)
			if err != nil {
				return nil, err
			}
			result.ModifiedFiles = append(result.ModifiedFiles, path)
			i = next
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if err := e.applyDelete(path); err != nil {
				return nil, err
			}
			result.ModifiedFiles = append(result.ModifiedFiles, path)
			i++
		case line == "":
			i++
		default:
			return nil, fmt.Errorf("unsupported patch section: %s", line)
		}
	}

	return result, nil
}

func (e *Engine) applyAdd(path string, lines []string, start int) (int, error) {
	fullPath, err := e.resolvePath(path)
	if err != nil {
		return 0, err
	}

	contentLines := make([]string, 0)
	i := start
	for ; i < len(lines)-1; i++ {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "*** ") {
			break
		}
		if strings.HasPrefix(line, "+") {
			contentLines = append(contentLines, strings.TrimPrefix(line, "+"))
		}
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0750); err != nil {
		return 0, err
	}
	if err := writeAtomically(fullPath, strings.Join(contentLines, "\n")); err != nil {
		return 0, err
	}

	return i, nil
}

func (e *Engine) applyUpdate(path string, lines []string, start int) (int, error) {
	fullPath, err := e.resolvePath(path)
	if err != nil {
		return 0, err
	}

	absRoot, err := filepath.Abs(e.repoRoot)
	if err != nil {
		return 0, err
	}
	relPath, err := filepath.Rel(absRoot, fullPath)
	if err != nil {
		return 0, err
	}
	rootFS := os.DirFS(absRoot)

	currentBytes, err := fs.ReadFile(rootFS, relPath)
	if err != nil {
		return 0, err
	}
	current := string(currentBytes)

	oldLines := make([]string, 0)
	newLines := make([]string, 0)
	i := start
	for ; i < len(lines)-1; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "*** ") {
			break
		}
		if strings.HasPrefix(line, "@@") {
			continue
		}
		if strings.HasPrefix(line, "-") {
			oldLines = append(oldLines, strings.TrimPrefix(line, "-"))
		}
		if strings.HasPrefix(line, "+") {
			newLines = append(newLines, strings.TrimPrefix(line, "+"))
		}
	}

	oldBlock := strings.Join(oldLines, "\n")
	newBlock := strings.Join(newLines, "\n")
	updated := current
	if oldBlock != "" {
		if !strings.Contains(current, oldBlock) {
			return 0, fmt.Errorf("update preimage not found for %s", path)
		}
		updated = strings.Replace(current, oldBlock, newBlock, 1)
	} else if newBlock != "" {
		updated = newBlock
	}

	if err := writeAtomically(fullPath, updated); err != nil {
		return 0, err
	}

	return i, nil
}

func (e *Engine) applyDelete(path string) error {
	fullPath, err := e.resolvePath(path)
	if err != nil {
		return err
	}
	return os.Remove(fullPath)
}

func (e *Engine) resolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	cleanPath := filepath.Clean(path)
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute path not allowed: %s", cleanPath)
	}
	absRoot, err := filepath.Abs(e.repoRoot)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(absRoot, cleanPath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path escapes repository root: %s", path)
	}
	return absPath, nil
}

func writeAtomically(path, content string) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".patch-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}
