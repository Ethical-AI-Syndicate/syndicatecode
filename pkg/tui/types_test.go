package tui

import "testing"

func TestReplayEventJSONTags_Bead_l3d_15_4(t *testing.T) {
	var event ReplayEvent
	if event.EventType != "" {
		t.Fatalf("expected zero-value event type, got %q", event.EventType)
	}
}

func TestPolicyDocumentType_Bead_l3d_15_4(t *testing.T) {
	policy := PolicyDocument{"version": "1.0.0"}
	if policy["version"] != "1.0.0" {
		t.Fatalf("expected version key to be preserved, got %+v", policy)
	}
}
