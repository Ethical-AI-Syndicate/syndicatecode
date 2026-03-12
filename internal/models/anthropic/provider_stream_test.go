package anthropic

import "testing"

func TestBuildToolInputDeltaEvent_UsesToolUseIDFromBlockIndex(t *testing.T) {
	toolUseIDByBlockIndex := map[int64]string{
		1: "tool-1",
		3: "tool-3",
	}

	event := buildToolInputDeltaEvent(3, `{"path":"/tmp/a.txt"}`, toolUseIDByBlockIndex)
	if event.ID != "tool-3" {
		t.Fatalf("expected tool ID tool-3, got %q", event.ID)
	}
	if event.Delta != `{"path":"/tmp/a.txt"}` {
		t.Fatalf("expected delta to round-trip, got %q", event.Delta)
	}
}
