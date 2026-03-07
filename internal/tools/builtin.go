package tools

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
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

		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		if !isPathInDir(absPath, allowedDir) {
			return nil, ErrPathNotAllowed
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, err
		}

		info, _ := os.Stat(absPath)

		return map[string]interface{}{
			"content":  string(content),
			"size":     info.Size(),
			"path":     absPath,
			"mod_time": info.ModTime().Unix(),
		}, nil
	}
}

func isPathInDir(path, dir string) bool {
	absPath, _ := filepath.Abs(path)
	absDir, _ := filepath.Abs(dir)
	return absPath == absDir || len(absPath) > len(absDir) && absPath[:len(absDir)+1] == absDir+"/"
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

		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		if !isPathInDir(absPath, allowedDir) {
			return nil, ErrPathNotAllowed
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
