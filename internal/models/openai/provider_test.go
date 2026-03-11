package openai

import (
	"context"
	"testing"

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
	p := NewProvider("test-key")
	m := p.Model("gpt-4o")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so goroutine exits fast
	ch, _ := m.Stream(ctx, models.Params{
		Messages: []models.Message{{Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: "hello"}}}},
	})
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
	// drain until closed
	for range ch {
	}
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

// TestBuildChatCompletionParamsSystemPrompt_Bead_l3d_16_4 verifies that a non-empty
// System field is prepended as the first message in the request params (Fix C1).
func TestBuildChatCompletionParamsSystemPrompt_Bead_l3d_16_4(t *testing.T) {
	p := models.Params{
		System: "You are a helpful assistant.",
		Messages: []models.Message{
			{Role: "user", Content: []models.ContentBlock{models.TextBlock{Text: "hello"}}},
		},
	}
	params := buildChatCompletionParams("gpt-4o", p)
	if len(params.Messages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(params.Messages))
	}
	// The first message should be a system message.
	first := params.Messages[0]
	if first.OfSystem == nil {
		t.Fatal("expected first message to be a system message")
	}
}
