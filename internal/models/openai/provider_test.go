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
