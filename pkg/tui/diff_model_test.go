package tui

import "testing"

func TestBuildRichDiffModelAndNavigation(t *testing.T) {
	events := []ReplayEvent{
		{EventType: "file_mutation", Payload: []byte(`{"path":"a.go","type":"update","hunks":[{"old_start":1,"old_lines":1,"new_start":1,"new_lines":1,"lines":["-old","+new"]}]}`)},
		{EventType: "file_mutation", Payload: []byte(`{"path":"b.go","type":"update","hunks":[{"old_start":3,"old_lines":1,"new_start":3,"new_lines":1,"lines":["-x","+y"]}]}`)},
	}

	model, err := BuildRichDiffModel(events)
	if err != nil {
		t.Fatalf("build model failed: %v", err)
	}
	if len(model.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(model.Files))
	}
	if model.CurrentFile() == nil || model.CurrentFile().Path != "a.go" {
		t.Fatalf("expected first sorted file a.go, got %+v", model.CurrentFile())
	}

	model.NextFile()
	if model.CurrentFile() == nil || model.CurrentFile().Path != "b.go" {
		t.Fatalf("expected b.go after next file, got %+v", model.CurrentFile())
	}
	model.PrevFile()
	if model.CurrentFile() == nil || model.CurrentFile().Path != "a.go" {
		t.Fatalf("expected a.go after prev file, got %+v", model.CurrentFile())
	}

	model.ToggleCurrentHunk()
	if !model.Files[model.FileCursor].Hunks[model.HunkCursor].Collapsed {
		t.Fatal("expected current hunk to become collapsed")
	}
}

func TestBuildRichDiffModel_MalformedHunksFallback(t *testing.T) {
	events := []ReplayEvent{
		{EventType: "file_mutation", Payload: []byte(`{"path":"a.go","type":"update","hunks":[{"old_start":"bad"}],"patch":"--- a/a.go\n+++ b/a.go\n@@ -1,1 +1,1 @@\n-old\n+new"}`)},
	}

	model, err := BuildRichDiffModel(events)
	if err != nil {
		t.Fatalf("build model failed: %v", err)
	}
	if len(model.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(model.Files))
	}
	if len(model.Files[0].Hunks) != 0 {
		t.Fatalf("expected malformed hunk to be ignored, got %+v", model.Files[0].Hunks)
	}
	if model.Files[0].RawPatch == "" {
		t.Fatal("expected patch fallback to be retained")
	}
}
