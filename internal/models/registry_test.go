package models

import (
	"context"
	"testing"
)

type mockProvider struct{}

func (m mockProvider) Name() string { return "mock" }
func (m mockProvider) Model(modelID string) LanguageModel {
	return &mockLanguageModel{id: modelID}
}

type mockLanguageModel struct {
	id string
}

func (m *mockLanguageModel) Stream(ctx context.Context, p Params) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}

func (m *mockLanguageModel) ModelID() string { return m.id }

func TestRegistry_NewRegistry_Bead_l3d_17_1(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.providers == nil {
		t.Error("providers map should not be nil")
	}
}

func TestRegistry_Register_Bead_l3d_17_1(t *testing.T) {
	r := NewRegistry()
	p := mockProvider{}

	r.Register("test-provider", p)

	r.mu.RLock()
	_, ok := r.providers["test-provider"]
	r.mu.RUnlock()

	if !ok {
		t.Error("provider should be registered")
	}
}

func TestRegistry_Resolve_Bead_l3d_17_1(t *testing.T) {
	r := NewRegistry()
	p := mockProvider{}

	r.Register("test-provider", p)

	lm, err := r.Resolve("test-provider", "test-model")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lm == nil {
		t.Error("expected non-nil LanguageModel")
	}
}

func TestRegistry_ResolveNotFound_Bead_l3d_17_1(t *testing.T) {
	r := NewRegistry()

	_, err := r.Resolve("nonexistent", "test-model")
	if err == nil {
		t.Error("expected error for unregistered provider")
	}
}
