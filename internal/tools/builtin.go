package tools

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func EchoHandler() ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		msg, _ := input["message"].(string)
		return map[string]interface{}{
			"output": msg,
		}, nil
	}
}

func ReadFileHandler(allowedDir string) ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		path, ok := input["path"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		absBase, err := filepath.Abs(allowedDir)
		if err != nil {
			return nil, err
		}

		absPath, err := resolvePathInDir(allowedDir, path)
		if err != nil {
			return nil, err
		}

		relPath, err := filepath.Rel(absBase, absPath)
		if err != nil {
			return nil, err
		}

		rootFS := os.DirFS(absBase)

		content, err := fs.ReadFile(rootFS, relPath)
		if err != nil {
			return nil, err
		}

		info, err := fs.Stat(rootFS, relPath)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"content":  string(content),
			"size":     info.Size(),
			"path":     absPath,
			"mod_time": info.ModTime().Unix(),
		}, nil
	}
}

func resolvePathInDir(dir, inputPath string) (string, error) {
	if inputPath == "" {
		return "", ErrInvalidInput
	}

	base, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(inputPath)
	if filepath.IsAbs(cleanPath) {
		return "", ErrPathNotAllowed
	}

	resolved := filepath.Join(base, cleanPath)
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(base, resolvedAbs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", ErrPathNotAllowed
	}

	return resolvedAbs, nil
}

var ErrInvalidInput = &ToolError{Message: "invalid input"}
var ErrPathNotAllowed = &ToolError{Message: "path not allowed"}

type ToolError struct {
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}

func WriteFileHandler(allowedDir string) ToolHandler {
	return func(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
		path, ok := input["path"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		content, ok := input["content"].(string)
		if !ok {
			return nil, ErrInvalidInput
		}

		absPath, err := resolvePathInDir(allowedDir, path)
		if err != nil {
			return nil, err
		}

		err = os.WriteFile(absPath, []byte(content), fs.FileMode(0644))
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"path":          absPath,
			"bytes_written": len(content),
		}, nil
	}
}
