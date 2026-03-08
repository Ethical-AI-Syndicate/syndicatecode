package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestBuildApprovalPayloadIncludesExecutionContext(t *testing.T) {
	t.Parallel()

	ctx := ExecutionContext{
		Executable:            "go",
		Args:                  []string{"test", "./..."},
		WorkingDir:            "/repo/internal",
		TimeoutSeconds:        15,
		EnvironmentExposure:   "allowlisted",
		NetworkPolicy:         NetworkPolicyAllowlisted,
		IsolationLevel:        IsolationLevel1,
		SideEffectClass:       SideEffectRead,
		ResourceMaxOutputB:    1024,
		ResourceMaxConcurrent: 2,
	}

	payload := BuildApprovalPayload("approval-1", ctx)
	if payload.ApprovalID != "approval-1" {
		t.Fatalf("expected approval id approval-1, got %s", payload.ApprovalID)
	}
	if payload.ExecutionContext.Executable != "go" {
		t.Fatalf("expected executable in payload")
	}
	if payload.ExecutionContext.IsolationLevel != IsolationLevel1 {
		t.Fatalf("expected isolation level in payload")
	}
}

func TestRecorderStoresExactExecutionContextMetadata(t *testing.T) {
	t.Parallel()

	recorder := NewInMemoryRecorder()
	now := time.Now().UTC()
	entry := ExecutionRecord{
		SessionID: "s1",
		TurnID:    "t1",
		ToolName:  "bash",
		Context: ExecutionContext{
			Executable:          "bash",
			Args:                []string{"-lc", "go test ./..."},
			WorkingDir:          "/repo",
			TimeoutSeconds:      30,
			EnvironmentExposure: "allowlisted",
			NetworkPolicy:       NetworkPolicyDisabled,
			IsolationLevel:      IsolationLevel2,
			SideEffectClass:     SideEffectShell,
		},
		RecordedAt: now,
	}

	if err := recorder.Record(context.Background(), entry); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	records := recorder.ListByTurn("t1")
	if len(records) != 1 {
		t.Fatalf("expected one recorded entry, got %d", len(records))
	}
	if records[0].Context.WorkingDir != "/repo" {
		t.Fatalf("expected exact working dir metadata")
	}
	if records[0].Context.TimeoutSeconds != 30 {
		t.Fatalf("expected timeout metadata to be preserved")
	}
}
