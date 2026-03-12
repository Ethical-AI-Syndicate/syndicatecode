package models

import (
	"context"
	"testing"
)

// MockProvider is a simple Provider implementation for testing.
type MockProvider struct {
	name   string
	models map[string]LanguageModel
}

func (mp *MockProvider) Name() string {
	return mp.name
}

func (mp *MockProvider) Model(id string) LanguageModel {
	return mp.models[id]
}

// MockLanguageModel is a simple LanguageModel implementation for testing.
type MockLanguageModel struct {
	id string
}

func (m *MockLanguageModel) Stream(ctx context.Context, p Params) (<-chan StreamEvent, error) {
	// Return an empty channel for testing
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}

func (m *MockLanguageModel) ModelID() string {
	return m.id
}

func TestNewRegistry_Bead_l3d_16_2(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegister_Bead_l3d_16_2(t *testing.T) {
	reg := NewRegistry()
	mock := &MockLanguageModel{id: "test-model"}
	provider := &MockProvider{name: "test-provider", models: map[string]LanguageModel{"test": mock}}

	reg.Register("test-provider", provider)

	// Verify registration by resolving
	model, err := reg.Resolve("test-provider", "test")
	if err != nil {
		t.Fatalf("Resolve failed after Register: %v", err)
	}
	if model == nil {
		t.Fatal("Resolve returned nil model")
	}
}

func TestResolve_Bead_l3d_16_2(t *testing.T) {
	reg := NewRegistry()
	mock := &MockLanguageModel{id: "my-model"}
	provider := &MockProvider{name: "my-provider", models: map[string]LanguageModel{"model1": mock}}

	reg.Register("my-provider", provider)

	model, err := reg.Resolve("my-provider", "model1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if model == nil {
		t.Fatal("Resolve returned nil model")
	}
}

func TestResolveUnregisteredProvider_Bead_l3d_16_2(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Resolve("nonexistent", "model1")
	if err == nil {
		t.Fatal("Resolve should fail for unregistered provider")
	}
}
