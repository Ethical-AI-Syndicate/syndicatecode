package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewServerFailsFastOnInvalidProviderPolicy_Bead_l3d_5_1(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "provider-policy.json")
	if err := os.WriteFile(invalidPath, []byte(`{"providers":[{"name":""}]}`), 0o600); err != nil {
		t.Fatalf("failed to write invalid provider policy: %v", err)
	}

	cfg := DefaultConfig()
	cfg.ProviderPolicyPath = invalidPath

	_, err := NewServer(context.Background(), cfg)
	if err == nil {
		t.Fatalf("expected NewServer to fail with invalid provider policy")
	}
	if !strings.Contains(err.Error(), "providers[0].name") {
		t.Fatalf("expected actionable validation error, got %v", err)
	}
}
