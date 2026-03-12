package audit

import "testing"

func TestIsKnownEventType_Bead_l3d_17_1(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{"known session started", EventSessionStarted, true},
		{"known turn completed", EventTurnCompleted, true},
		{"unknown type", "unknown_event", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownEventType(tt.eventType); got != tt.want {
				t.Errorf("IsKnownEventType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKnownEventTypes_Bead_l3d_17_1(t *testing.T) {
	types := KnownEventTypes()
	if len(types) == 0 {
		t.Error("KnownEventTypes() should return non-empty slice")
	}
}
