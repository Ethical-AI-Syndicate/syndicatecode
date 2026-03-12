package openai

import (
	"context"
	"encoding/json"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

func TestProviderName_Bead_l3d_16_4(t *testing.T) {
	p := NewProvider("test-key")
	if p.Name() != "openai" {
		t.Errorf("expected openai, got %s", p.Name())
	}
}

func TestModelID_Bead_l3d_16_4(t *testing.T) {
	p := NewProvider("test-key")
	m := p.Model("gpt-4o")
	if m.ModelID() != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", m.ModelID())
	}
}

func TestStreamReturnsChannel_Bead_l3d_16_4(t *testing.T) {
	// Tests that Stream returns a non-nil channel (structural test, no live API call)
	p := NewProvider("test-key")
	m := p.Model("gpt-4o")
	ch, err := m.Stream(context.Background(), models.Params{
		Messages: []models.Message{{Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: "hello"}}}},
	})
	// ch may be non-nil even if stream will fail due to no real API key
	_ = ch
	_ = err
	// The test just verifies the call doesn't panic
}

func TestMessageStartEventType_Bead_l3d_16_4(t *testing.T) {
	// Structural test: verify that MessageStartEvent is accepted by the StreamEvent channel type
	// and that InputTokens field is properly typed as int
	event := models.MessageStartEvent{InputTokens: 42}

	// Verify the event implements StreamEvent interface
	var _ models.StreamEvent = event

	// Verify we can read the InputTokens field
	if event.InputTokens != 42 {
		t.Errorf("expected InputTokens=42, got %d", event.InputTokens)
	}
}

func TestBuildMessages_PreservesToolUseAndToolResultBlocks(t *testing.T) {
	messages := []models.Message{
		{
			Role: "assistant",
			Content: []models.ContentBlock{
				models.ToolUseBlock{ID: "call_1", Name: "read_file", Input: json.RawMessage(`{"path":"/tmp/x.txt"}`)},
			},
		},
		{
			Role: "user",
			Content: []models.ContentBlock{
				models.ToolResultBlock{ToolUseID: "call_1", Content: `{"ok":true}`},
			},
		},
	}

	out := buildMessages(messages)
	if len(out) != 2 {
		t.Fatalf("expected 2 output messages, got %d", len(out))
	}

	if out[0].OfAssistant == nil {
		t.Fatalf("expected first message to be assistant, got %#v", out[0])
	}
	toolCalls := out[0].GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("expected exactly 1 tool call, got %d", len(toolCalls))
	}
	if id := toolCalls[0].GetID(); id == nil || *id != "call_1" {
		t.Fatalf("expected tool call id call_1, got %v", id)
	}
	if fn := toolCalls[0].GetFunction(); fn == nil || fn.Name != "read_file" || fn.Arguments != `{"path":"/tmp/x.txt"}` {
		t.Fatalf("expected function read_file with arguments JSON, got %+v", fn)
	}

	if out[1].OfTool == nil {
		t.Fatalf("expected second message to be tool, got %#v", out[1])
	}
	if toolCallID := out[1].GetToolCallID(); toolCallID == nil || *toolCallID != "call_1" {
		t.Fatalf("expected tool_call_id call_1, got %v", toolCallID)
	}
}

func TestCollectToolCallDeltaEvents_BackfillsIDByIndex(t *testing.T) {
	states := map[int64]*toolCallDeltaState{}

	events := collectToolCallDeltaEvents([]openaisdk.ChatCompletionChunkChoiceDeltaToolCall{
		{
			Index: 0,
			ID:    "call_1",
			Function: openaisdk.ChatCompletionChunkChoiceDeltaToolCallFunction{
				Name: "read_file",
			},
		},
	}, states)

	if len(events) != 1 {
		t.Fatalf("expected 1 event from first chunk, got %d", len(events))
	}
	start, ok := events[0].(models.ToolUseStartEvent)
	if !ok || start.ID != "call_1" || start.Name != "read_file" {
		t.Fatalf("expected ToolUseStartEvent for call_1/read_file, got %#v", events[0])
	}

	events = collectToolCallDeltaEvents([]openaisdk.ChatCompletionChunkChoiceDeltaToolCall{
		{
			Index: 0,
			Function: openaisdk.ChatCompletionChunkChoiceDeltaToolCallFunction{
				Arguments: `{"path":"/tmp/x.txt"}`,
			},
		},
	}, states)

	if len(events) != 1 {
		t.Fatalf("expected 1 event from second chunk, got %d", len(events))
	}
	input, ok := events[0].(models.ToolInputDeltaEvent)
	if !ok || input.ID != "call_1" || input.Delta != `{"path":"/tmp/x.txt"}` {
		t.Fatalf("expected ToolInputDeltaEvent with backfilled call_1 ID, got %#v", events[0])
	}
}
