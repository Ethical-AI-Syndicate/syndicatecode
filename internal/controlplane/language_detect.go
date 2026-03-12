package controlplane

import (
	"os"
	"path/filepath"
	"strings"
)

type detectedLanguage struct {
	Language   string
	Executable string
}

func detectLanguage(repoRoot, path string) detectedLanguage {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return detectedLanguage{Language: "go", Executable: "gopls"}
	case ".ts", ".tsx", ".js", ".jsx":
		return detectedLanguage{Language: "typescript", Executable: "typescript-language-server"}
	case ".py":
		return detectedLanguage{Language: "python", Executable: "pylsp"}
	}

	if fileExists(filepath.Join(repoRoot, "go.mod")) {
		return detectedLanguage{Language: "go", Executable: "gopls"}
	}
	if fileExists(filepath.Join(repoRoot, "package.json")) {
		return detectedLanguage{Language: "typescript", Executable: "typescript-language-server"}
	}
	if fileExists(filepath.Join(repoRoot, "pyproject.toml")) {
		return detectedLanguage{Language: "python", Executable: "pylsp"}
	}

	return detectedLanguage{Language: "text", Executable: ""}
}

func fileExists(path string) bool {
	// #nosec G703 // os.Stat is safe here - only checks if file exists
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
