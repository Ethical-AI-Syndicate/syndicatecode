package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/patch"
)

func TestApplyPatchHandler(t *testing.T) {
	repoRoot := t.TempDir()
	engine := patch.NewEngine(repoRoot)
	handler := ApplyPatchHandler(engine, nil)

	result, err := handler(context.Background(), map[string]interface{}{
		"patch": "*** Begin Patch\n*** Add File: src/a.txt\n+hello\n*** End Patch",
	})
	if err != nil {
		t.Fatalf("apply patch handler failed: %v", err)
	}

	files, ok := result["files_modified"].([]string)
	if !ok || len(files) != 1 || files[0] != "src/a.txt" {
		t.Fatalf("unexpected files modified: %+v", result["files_modified"])
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, "src", "a.txt"))
	if err != nil {
		t.Fatalf("expected file to be created: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected file content: %q", string(content))
	}
}
