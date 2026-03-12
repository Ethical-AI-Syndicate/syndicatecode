package controlplane

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLanguage_ByExtension(t *testing.T) {
	cases := []struct {
		path     string
		language string
		exec     string
	}{
		{path: "main.go", language: "go", exec: "gopls"},
		{path: "main.ts", language: "typescript", exec: "typescript-language-server"},
		{path: "main.py", language: "python", exec: "pylsp"},
	}

	for _, tc := range cases {
		got := detectLanguage(t.TempDir(), tc.path)
		if got.Language != tc.language || got.Executable != tc.exec {
			t.Fatalf("detectLanguage(%q)=%+v, want language=%s exec=%s", tc.path, got, tc.language, tc.exec)
		}
	}
}

func TestDetectLanguage_ByRepoMarkers(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module x"), 0600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if got := detectLanguage(repo, "unknown.xyz"); got.Language != "go" {
		t.Fatalf("expected go fallback from marker, got %+v", got)
	}
}

func TestDetectLanguage_UnknownFallback(t *testing.T) {
	got := detectLanguage(t.TempDir(), "README.unknown")
	if got.Language != "text" || got.Executable != "" {
		t.Fatalf("expected text fallback, got %+v", got)
	}
}
