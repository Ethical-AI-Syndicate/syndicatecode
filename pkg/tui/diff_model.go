package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type DiffHunk struct {
	OldStart  int
	OldLines  int
	NewStart  int
	NewLines  int
	Lines     []string
	Collapsed bool
}

type DiffFile struct {
	Path       string
	MutType    string
	BeforeHash string
	AfterHash  string
	Hunks      []DiffHunk
	RawPatch   string
}

type RichDiffModel struct {
	Files      []DiffFile
	FileCursor int
	HunkCursor int
}

func BuildRichDiffModel(events []ReplayEvent) (*RichDiffModel, error) {
	filesByPath := make(map[string]DiffFile)
	order := make([]string, 0)

	for _, ev := range events {
		if ev.EventType != "file_mutation" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			continue
		}
		path, _ := payload["path"].(string)
		if path == "" {
			path = "(unknown-file)"
		}
		current, exists := filesByPath[path]
		if !exists {
			order = append(order, path)
			current = DiffFile{Path: path}
		}
		if current.MutType == "" {
			current.MutType, _ = payload["type"].(string)
		}
		if current.BeforeHash == "" {
			current.BeforeHash, _ = payload["before_hash"].(string)
		}
		if current.AfterHash == "" {
			current.AfterHash, _ = payload["after_hash"].(string)
		}
		if patch, ok := payload["patch"].(string); ok && strings.TrimSpace(patch) != "" {
			current.RawPatch = patch
		}
		if hunks := parsePayloadHunks(payload); len(hunks) > 0 {
			current.Hunks = append(current.Hunks, hunks...)
		}
		filesByPath[path] = current
	}

	if len(filesByPath) == 0 {
		return &RichDiffModel{}, nil
	}

	sort.Strings(order)
	files := make([]DiffFile, 0, len(order))
	for _, path := range order {
		files = append(files, filesByPath[path])
	}

	return &RichDiffModel{Files: files}, nil
}

func parsePayloadHunks(payload map[string]interface{}) []DiffHunk {
	rawHunks, ok := payload["hunks"].([]interface{})
	if !ok {
		return nil
	}
	hunks := make([]DiffHunk, 0, len(rawHunks))
	for _, raw := range rawHunks {
		hunkMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		oldStart, okOldStart := jsonNumberToInt(hunkMap["old_start"])
		oldLines, okOldLines := jsonNumberToInt(hunkMap["old_lines"])
		newStart, okNewStart := jsonNumberToInt(hunkMap["new_start"])
		newLines, okNewLines := jsonNumberToInt(hunkMap["new_lines"])
		if !okOldStart || !okOldLines || !okNewStart || !okNewLines {
			continue
		}
		lines := make([]string, 0)
		if rawLines, ok := hunkMap["lines"].([]interface{}); ok {
			for _, entry := range rawLines {
				line, ok := entry.(string)
				if ok {
					lines = append(lines, line)
				}
			}
		}
		hunks = append(hunks, DiffHunk{OldStart: oldStart, OldLines: oldLines, NewStart: newStart, NewLines: newLines, Lines: lines})
	}
	return hunks
}

func (m *RichDiffModel) NextFile() {
	if len(m.Files) == 0 {
		return
	}
	m.FileCursor = (m.FileCursor + 1) % len(m.Files)
	m.HunkCursor = 0
}

func (m *RichDiffModel) PrevFile() {
	if len(m.Files) == 0 {
		return
	}
	m.FileCursor--
	if m.FileCursor < 0 {
		m.FileCursor = len(m.Files) - 1
	}
	m.HunkCursor = 0
}

func (m *RichDiffModel) NextHunk() {
	file := m.CurrentFile()
	if file == nil || len(file.Hunks) == 0 {
		return
	}
	m.HunkCursor = (m.HunkCursor + 1) % len(file.Hunks)
}

func (m *RichDiffModel) PrevHunk() {
	file := m.CurrentFile()
	if file == nil || len(file.Hunks) == 0 {
		return
	}
	m.HunkCursor--
	if m.HunkCursor < 0 {
		m.HunkCursor = len(file.Hunks) - 1
	}
}

func (m *RichDiffModel) ToggleCurrentHunk() {
	file := m.CurrentFile()
	if file == nil || len(file.Hunks) == 0 || m.HunkCursor >= len(file.Hunks) {
		return
	}
	file.Hunks[m.HunkCursor].Collapsed = !file.Hunks[m.HunkCursor].Collapsed
	m.Files[m.FileCursor] = *file
}

func (m *RichDiffModel) CurrentFile() *DiffFile {
	if len(m.Files) == 0 || m.FileCursor < 0 || m.FileCursor >= len(m.Files) {
		return nil
	}
	copy := m.Files[m.FileCursor]
	return &copy
}

func (m *RichDiffModel) Render() string {
	if len(m.Files) == 0 {
		return "(no file mutation events)"
	}
	var b strings.Builder
	for i, file := range m.Files {
		activeMarker := " "
		if i == m.FileCursor {
			activeMarker = ">"
		}
		fmt.Fprintf(&b, "%s %s (%s)\n", activeMarker, file.Path, file.MutType)
		if file.BeforeHash != "" || file.AfterHash != "" {
			fmt.Fprintf(&b, "  hashes: %s -> %s\n", file.BeforeHash, file.AfterHash)
		}
		if len(file.Hunks) == 0 {
			if strings.TrimSpace(file.RawPatch) != "" {
				fmt.Fprintf(&b, "  patch:\n%s\n", colorizeUnifiedDiff(file.RawPatch))
			} else {
				b.WriteString("  (no structured hunks)\n")
			}
			continue
		}
		for h := range file.Hunks {
			hunk := file.Hunks[h]
			hActive := " "
			if i == m.FileCursor && h == m.HunkCursor {
				hActive = "*"
			}
			fmt.Fprintf(&b, "  %s @@ -%d,%d +%d,%d @@", hActive, hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
			if hunk.Collapsed {
				b.WriteString(" [collapsed]\n")
				continue
			}
			b.WriteString("\n")
			for _, line := range hunk.Lines {
				b.WriteString("    ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
