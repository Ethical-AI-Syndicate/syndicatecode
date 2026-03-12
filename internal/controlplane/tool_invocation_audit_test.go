package controlplane

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

func TestAuditExecutionRecorder_EmitsTurnAndApprovalMetadata(t *testing.T) {
	store, err := audit.NewEventStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recorder := &auditExecutionRecorder{store: store}
	call := tools.ToolCall{
		ID:        "call-1",
		ToolName:  "echo",
		SessionID: "sess-1",
		Input: map[string]interface{}{
			"turn_id":     "turn-1",
			"approval_id": "appr-1",
		},
	}

	recorder.BeforeExecute(context.Background(), call, tools.ToolDefinition{Name: "echo"})
	recorder.AfterExecute(context.Background(), call, tools.ToolDefinition{Name: "echo"}, &tools.ToolResult{Success: true, Output: map[string]interface{}{"ok": true}}, nil, 5*time.Millisecond)

	events, err := store.QueryBySession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("query events failed: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected tool invocation events, got %d", len(events))
	}

	var foundResult bool
	for _, evt := range events {
		if evt.EventType != audit.EventToolResult {
			continue
		}
		foundResult = true
		if evt.TurnID != "turn-1" {
			t.Fatalf("expected event turn id turn-1, got %q", evt.TurnID)
		}
		var payload map[string]string
		if err := json.Unmarshal(evt.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["approval_id"] != "appr-1" {
			t.Fatalf("expected approval id appr-1, got %q", payload["approval_id"])
		}
		if payload["output_ref"] == "" {
			t.Fatal("expected output_ref in tool_result payload")
		}
	}
	if !foundResult {
		t.Fatal("expected tool_result event")
	}
}
