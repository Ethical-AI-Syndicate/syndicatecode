package models_test

import (
	"context"
	"testing"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
)

func TestContentBlockTypes_Bead_l3d_16_1(t *testing.T) {
	// Verify sealed interface works correctly
	var _ models.ContentBlock = models.TextBlock{Text: "hello"}
	var _ models.ContentBlock = models.ToolUseBlock{ID: "1", Name: "read_file"}
	var _ models.ContentBlock = models.ToolResultBlock{ToolUseID: "1", Content: "ok"}
}

func TestStreamEventTypes_Bead_l3d_16_1(t *testing.T) {
	var _ models.StreamEvent = models.TextDeltaEvent{Delta: "hi"}
	var _ models.StreamEvent = models.ToolUseStartEvent{ID: "1", Name: "read_file"}
	var _ models.StreamEvent = models.MessageDeltaEvent{StopReason: "end_turn"}
	var _ models.StreamEvent = models.MessageStartEvent{InputTokens: 100}
	var _ models.StreamEvent = models.ToolInputDeltaEvent{ID: "1", Delta: "{}"}
}

func TestLanguageModelInterface_Bead_l3d_16_1(t *testing.T) {
	// Verify a mock can implement the interface
	var _ models.LanguageModel = &mockModel{}
	var _ models.Provider = &mockProvider{}
}

// minimal mocks — just for interface compliance
type mockModel struct{}

func (m *mockModel) ModelID() string { return "mock" }
func (m *mockModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent)
	close(ch)
	return ch, nil
}

type mockProvider struct{}

func (p *mockProvider) Name() string { return "mock" }
func (p *mockProvider) Model(_ string) models.LanguageModel { return &mockModel{} }
