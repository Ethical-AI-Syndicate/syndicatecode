package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltin_Echo(t *testing.T) {
	handler := EchoHandler()

	result, err := handler(context.Background(), map[string]interface{}{
		"message": "test",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["output"] != "test" {
		t.Errorf("got %v, want 'test'", result["output"])
	}
}

func TestBuiltin_ReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	handler := ReadFileHandler(tmpDir)

	path := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	result, err := handler(context.Background(), map[string]interface{}{
		"path": "sample.txt",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["content"] != "hello world" {
		t.Errorf("got %v, want 'hello world'", result["content"])
	}
}

func TestBuiltin_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	handler := WriteFileHandler(tmpDir)
	path := filepath.Join(tmpDir, "output.txt")

	result, err := handler(context.Background(), map[string]interface{}{
		"path":    "output.txt",
		"content": "hello world",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["bytes_written"] != 11 {
		t.Errorf("got %v, want 11", result["bytes_written"])
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("got %v, want 'hello world'", string(content))
	}
}
