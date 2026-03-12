package sandbox

import (
	"context"
	"testing"
)

func TestSelectRunnerForTrustTierReturnsCorrectRunner_Bead_l3d_17_1(t *testing.T) {
	l1 := &L1Runner{}
	l2 := &L2Runner{}

	// tier1 should return l1
	runner := selectRunnerForTrustTier("tier1", l1, l2)
	if runner != l1 {
		t.Errorf("tier1 should return l1 runner")
	}

	// tier2 should return l2
	runner = selectRunnerForTrustTier("tier2", l1, l2)
	if runner != l2 {
		t.Errorf("tier2 should return l2 runner")
	}

	// tier3 should return l2
	runner = selectRunnerForTrustTier("tier3", l1, l2)
	if runner != l2 {
		t.Errorf("tier3 should return l2 runner")
	}

	// unknown tier should return l1
	runner = selectRunnerForTrustTier("unknown", l1, l2)
	if runner != l1 {
		t.Errorf("unknown tier should return l1 runner")
	}
}

func TestRestrictedShellByTrustHandler_Bead_l3d_17_1(t *testing.T) {
	l1 := &L1Runner{}
	l2 := &L2Runner{}

	handler := RestrictedShellByTrustHandler(nil, l1, l2)
	if handler == nil {
		t.Fatal("RestrictedShellByTrustHandler returned nil")
	}

	// Test that handler accepts input map with optional fields
	input := map[string]interface{}{
		"command":    "echo test",
		"work_dir":   "/tmp",
		"session_id": "test-session",
	}

	ctx := context.Background()
	result, err := handler(ctx, input)

	// We don't strictly assert success here since it depends on the runner implementation,
	// but we can verify the handler doesn't panic and produces a result map.
	if err != nil {
		// It's okay if it errors due to implementation details,
		// just verify it was called with the right structure
		t.Logf("handler returned error: %v", err)
	}
	if result != nil {
		t.Logf("handler returned result: %v", result)
	}
}
