package anthropic_test

import (
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
	anth "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models/anthropic"
)

func TestAnthropicProvider_Name_Bead_l3d_16_3(t *testing.T) {
	p := anth.NewProvider("test-key")
	if p.Name() != "anthropic" {
		t.Errorf("got name %q, want %q", p.Name(), "anthropic")
	}
}

func TestAnthropicProvider_ModelID_Bead_l3d_16_3(t *testing.T) {
	p := anth.NewProvider("test-key")
	m := p.Model("claude-sonnet-4-6")
	if m.ModelID() != "claude-sonnet-4-6" {
		t.Errorf("got model ID %q, want %q", m.ModelID(), "claude-sonnet-4-6")
	}
}

func TestAnthropicModel_ImplementsInterface_Bead_l3d_16_3(t *testing.T) {
	p := anth.NewProvider("test-key")
	m := p.Model("claude-sonnet-4-6")
	if m == nil {
		t.Fatal("expected non-nil LanguageModel")
	}
	var _ models.Provider = p
}
