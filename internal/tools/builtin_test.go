package tools

import (
	"context"
	"os"
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
	handler := ReadFileHandler("/tmp")

	f, err := os.CreateTemp("/tmp", "test")
	if err != nil {
		t.Skip("cannot create temp file")
	}
	path := f.Name()
	t.Cleanup(func() { _ = os.Remove(path) })

	if _, err := f.WriteString("hello world"); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	result, err := handler(context.Background(), map[string]interface{}{
		"path": path,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["content"] != "hello world" {
		t.Errorf("got %v, want 'hello world'", result["content"])
	}
}

func TestBuiltin_WriteFile(t *testing.T) {
	handler := WriteFileHandler("/tmp")

	f, err := os.CreateTemp("/tmp", "testwrite")
	if err != nil {
		t.Skip("cannot create temp file")
	}
	path := f.Name()
	t.Cleanup(func() { _ = os.Remove(path) })

	if err := f.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	result, err := handler(context.Background(), map[string]interface{}{
		"path":    path,
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
