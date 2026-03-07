package patch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEngine_ApplyAddUpdateDelete(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewEngine(repoRoot)

	addPatch := "*** Begin Patch\n*** Add File: notes.txt\n+hello\n+world\n*** End Patch"
	result, err := engine.Apply(addPatch)
	if err != nil {
		t.Fatalf("add apply failed: %v", err)
	}
	if len(result.ModifiedFiles) != 1 || result.ModifiedFiles[0] != "notes.txt" {
		t.Fatalf("unexpected modified files: %+v", result.ModifiedFiles)
	}

	content, err := os.ReadFile(filepath.Join(repoRoot, "notes.txt"))
	if err != nil {
		t.Fatalf("failed reading added file: %v", err)
	}
	if string(content) != "hello\nworld" {
		t.Fatalf("unexpected add content: %q", string(content))
	}

	updatePatch := "*** Begin Patch\n*** Update File: notes.txt\n@@\n-hello\n+hi\n*** End Patch"
	if _, err := engine.Apply(updatePatch); err != nil {
		t.Fatalf("update apply failed: %v", err)
	}

	updated, err := os.ReadFile(filepath.Join(repoRoot, "notes.txt"))
	if err != nil {
		t.Fatalf("failed reading updated file: %v", err)
	}
	if string(updated) != "hi\nworld" {
		t.Fatalf("unexpected updated content: %q", string(updated))
	}

	deletePatch := "*** Begin Patch\n*** Delete File: notes.txt\n*** End Patch"
	if _, err := engine.Apply(deletePatch); err != nil {
		t.Fatalf("delete apply failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file to be deleted")
	}
}

func TestEngine_RejectsPathTraversal(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewEngine(repoRoot)

	patch := "*** Begin Patch\n*** Add File: ../escape.txt\n+bad\n*** End Patch"
	if _, err := engine.Apply(patch); err == nil {
		t.Fatal("expected traversal patch to be rejected")
	}
}

func TestEngine_RejectsMalformedPatch(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewEngine(repoRoot)

	patch := "*** Add File: notes.txt\n+no envelope"
	if _, err := engine.Apply(patch); err == nil {
		t.Fatal("expected malformed patch to be rejected")
	}
}
