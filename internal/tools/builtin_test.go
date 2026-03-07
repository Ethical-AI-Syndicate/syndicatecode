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
	defer os.Remove(f.Name())
	f.WriteString("hello world")
	f.Close()

	result, err := handler(context.Background(), map[string]interface{}{
		"path": f.Name(),
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
	f.Close()
	os.Remove(path)

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

	content, _ := os.ReadFile(path)
	if string(content) != "hello world" {
		t.Errorf("got %v, want 'hello world'", string(content))
	}

	os.Remove(path)
}
