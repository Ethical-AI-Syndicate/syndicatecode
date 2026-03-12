package agent_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/agent"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/models"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/tools"
)

type mockModel struct {
	events []models.StreamEvent
	id     string
}

func (m *mockModel) ModelID() string { return m.id }

func (m *mockModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	ch := make(chan models.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

type transientErr struct{}

func (transientErr) Error() string   { return "temporary provider outage" }
func (transientErr) Temporary() bool { return true }

type flakyModel struct {
	id          string
	failures    int
	attempts    int
	attemptsMux sync.Mutex
}

func (m *flakyModel) ModelID() string { return m.id }

func (m *flakyModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	m.attemptsMux.Lock()
	defer m.attemptsMux.Unlock()
	m.attempts++
	if m.attempts <= m.failures {
		return nil, transientErr{}
	}
	ch := make(chan models.StreamEvent, 4)
	ch <- models.TextDeltaEvent{Delta: "ok"}
	ch <- models.MessageDeltaEvent{StopReason: "end_turn", OutputTokens: 1}
	close(ch)
	return ch, nil
}

type blockingModel struct{ id string }

func (m *blockingModel) ModelID() string { return m.id }

func (m *blockingModel) Stream(ctx context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type idleNeverCloseModel struct{ id string }

func (m *idleNeverCloseModel) ModelID() string { return m.id }

func (m *idleNeverCloseModel) Stream(_ context.Context, _ models.Params) (<-chan models.StreamEvent, error) {
	return make(chan models.StreamEvent), nil
}

type captureEmitter struct {
	mu     sync.Mutex
	events []agent.AgentEvent
}

func (c *captureEmitter) Emit(_ string, e agent.AgentEvent) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func (c *captureEmitter) Snapshot() []agent.AgentEvent {
	c.mu.Lock()
	defer c.mu.Unlock()

	events := make([]agent.AgentEvent, len(c.events))
	copy(events, c.events)
	return events
}

type noopGate struct{}

func (noopGate) RequestApproval(_ context.Context, _ tools.ToolCall) (agent.ApprovalResult, error) {
	return agent.ApprovalResult{Approved: true}, nil
}

type pendingApprovalGate struct{}

func (pendingApprovalGate) RequestApproval(_ context.Context, _ tools.ToolCall) (agent.ApprovalResult, error) {
	return agent.ApprovalResult{Approved: false, Reason: "approval_required", ApprovalID: "appr-123"}, nil
}

func TestRunner_TextOnlyTurn_Bead_l3d_X_1(t *testing.T) {
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.TextDeltaEvent{Delta: "Hello"},
			models.TextDeltaEvent{Delta: " world"},
			models.MessageDeltaEvent{StopReason: "end_turn", OutputTokens: 10},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg, nil)

	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, agent.DefaultConfig("tier1"))
	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t1", SessionID: "s1", Message: "say hi", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	completed := false
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				goto drained
			}
			if evt.Type == agent.EventTurnCompleted {
				completed = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for turn completion")
		}
	}

drained:
	if !completed {
		t.Fatal("expected turn_completed before channel close")
	}

	textCount := 0
	for _, e := range emitter.Snapshot() {
		if e.Type == agent.EventTextDelta {
			textCount++
		}
	}
	if textCount != 2 {
		t.Errorf("expected 2 text delta events, got %d", textCount)
	}
}

func TestRunner_MaxDepthEnforced_Bead_l3d_X_1(t *testing.T) {
	toolInput, _ := json.Marshal(map[string]string{"name": "test"})
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.ToolUseStartEvent{ID: "tu1", Name: "echo"},
			models.ToolInputDeltaEvent{ID: "tu1", Delta: string(toolInput)},
			models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 5},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	if err := reg.Register(tools.ToolDefinition{
		Name:             "echo",
		Version:          "1",
		SideEffect:       tools.SideEffectNone,
		ApprovalRequired: false,
		InputSchema:      map[string]tools.FieldSchema{"name": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"name": {Type: "string"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	exec := tools.NewExecutor(reg, map[string]tools.ToolHandler{
		"echo": func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return input, nil
		},
	})

	cfg := agent.DefaultConfig("tier0")
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)
	ch, _ := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t2", SessionID: "s1", Message: "loop", TrustTier: "tier0"})

	stopReason := ""
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				goto done
			}
			if evt.Type == agent.EventTurnCompleted {
				stopReason = evt.StopReason
				goto done
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
done:
	if stopReason != "max_depth" {
		t.Errorf("expected stop_reason max_depth, got %q", stopReason)
	}
}

func TestRunner_EnforcesTurnTimeout_Bead_l3d_X_2(t *testing.T) {
	m := &blockingModel{id: "mock"}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg, nil)

	cfg := agent.DefaultConfig("tier1")
	cfg.TurnTimeout = 20 * time.Millisecond
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-timeout", SessionID: "s1", Message: "slow", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	evt := waitForTerminalEvent(t, ch)
	if evt.Type != agent.EventTurnFailed {
		t.Fatalf("expected terminal event turn_failed, got %q", evt.Type)
	}
	if evt.StopReason != "timeout" {
		t.Fatalf("expected stop_reason timeout, got %q (data=%q)", evt.StopReason, evt.Data)
	}
}

func TestRunner_EnforcesTurnTimeout_WhenStreamIdleNeverCloses_Bead_l3d_X_2(t *testing.T) {
	m := &idleNeverCloseModel{id: "mock"}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg, nil)

	cfg := agent.DefaultConfig("tier1")
	cfg.TurnTimeout = 20 * time.Millisecond
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-timeout-idle", SessionID: "s1", Message: "idle", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	evt := waitForTerminalEvent(t, ch)
	if evt.Type != agent.EventTurnFailed {
		t.Fatalf("expected terminal event turn_failed, got %q", evt.Type)
	}
	if evt.StopReason != "timeout" {
		t.Fatalf("expected stop_reason timeout, got %q (data=%q)", evt.StopReason, evt.Data)
	}
}

func TestRunner_RetriesTransientModelErrors_Bead_l3d_X_2(t *testing.T) {
	m := &flakyModel{id: "mock", failures: 2}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg, nil)

	cfg := agent.DefaultConfig("tier1")
	cfg.MaxRetries = 2
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-retry", SessionID: "s1", Message: "retry", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	evt := waitForTerminalEvent(t, ch)
	if evt.Type != agent.EventTurnCompleted {
		t.Fatalf("expected terminal event turn_completed, got %q", evt.Type)
	}
	if evt.StopReason != "end_turn" {
		t.Fatalf("expected stop_reason end_turn, got %q", evt.StopReason)
	}

	m.attemptsMux.Lock()
	attempts := m.attempts
	m.attemptsMux.Unlock()
	if attempts != 3 {
		t.Fatalf("expected 3 stream attempts, got %d", attempts)
	}
}

func TestRunner_StopsOnMaxOutputBytes_Bead_l3d_X_2(t *testing.T) {
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.TextDeltaEvent{Delta: "hello"},
			models.MessageDeltaEvent{StopReason: "end_turn", OutputTokens: 5},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	exec := tools.NewExecutor(reg, nil)

	cfg := agent.DefaultConfig("tier1")
	cfg.MaxOutputBytes = 4
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-output", SessionID: "s1", Message: "big", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	evt := waitForTerminalEvent(t, ch)
	if evt.Type != agent.EventTurnCompleted {
		t.Fatalf("expected terminal event turn_completed, got %q", evt.Type)
	}
	if evt.StopReason != "max_output_bytes" {
		t.Fatalf("expected stop_reason max_output_bytes, got %q", evt.StopReason)
	}
}

func TestRunner_StopsTurnWhenApprovalRequired_Bead_t10_5(t *testing.T) {
	toolInput, _ := json.Marshal(map[string]string{"path": "a.txt", "content": "hello"})
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.ToolUseStartEvent{ID: "tu-approval", Name: "write_file"},
			models.ToolInputDeltaEvent{ID: "tu-approval", Delta: string(toolInput)},
			models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 5},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	if err := reg.Register(tools.ToolDefinition{
		Name:             "write_file",
		Version:          "1",
		SideEffect:       tools.SideEffectWrite,
		Security:         tools.SecurityMetadata{FilesystemScope: "repo"},
		ApprovalRequired: true,
		InputSchema:      map[string]tools.FieldSchema{"path": {Type: "string"}, "content": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"bytes_written": {Type: "integer"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	exec := tools.NewExecutor(reg, nil)

	runner := agent.NewRunner(m, reg, exec, pendingApprovalGate{}, emitter, agent.DefaultConfig("tier1"))
	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-await", SessionID: "s1", Message: "write", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	var awaiting agent.AgentEvent
	var completed agent.AgentEvent
	timeout := time.After(2 * time.Second)
	for completed.Type == "" {
		select {
		case evt, ok := <-ch:
			if !ok {
				t.Fatal("channel closed before terminal event")
			}
			if evt.Type == agent.EventAwaitingApproval {
				awaiting = evt
			}
			if evt.Type == agent.EventTurnCompleted {
				completed = evt
			}
		case <-timeout:
			t.Fatal("timed out waiting for awaiting_approval terminal flow")
		}
	}

	if awaiting.Type != agent.EventAwaitingApproval {
		t.Fatalf("expected awaiting_approval event, got %+v", awaiting)
	}
	if awaiting.StopReason != "awaiting_approval" {
		t.Fatalf("expected awaiting_approval stop reason on event, got %q", awaiting.StopReason)
	}
	if awaiting.ToolName != "write_file" {
		t.Fatalf("expected tool_name write_file, got %q", awaiting.ToolName)
	}
	if awaiting.ApprovalID != "appr-123" {
		t.Fatalf("expected approval_id appr-123, got %q", awaiting.ApprovalID)
	}
	if completed.StopReason != "awaiting_approval" {
		t.Fatalf("expected terminal stop_reason awaiting_approval, got %q", completed.StopReason)
	}
}

func TestRunner_InvalidToolInputStopsTurnWithExplicitReason(t *testing.T) {
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.ToolUseStartEvent{ID: "tu-invalid", Name: "echo"},
			models.ToolInputDeltaEvent{ID: "tu-invalid", Delta: `{"message":`},
			models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 1},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	if err := reg.Register(tools.ToolDefinition{
		Name:             "echo",
		Version:          "1",
		SideEffect:       tools.SideEffectNone,
		ApprovalRequired: false,
		InputSchema:      map[string]tools.FieldSchema{"message": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"message": {Type: "string"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	exec := tools.NewExecutor(reg, nil)
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, agent.DefaultConfig("tier1"))

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-invalid", SessionID: "s1", Message: "bad input", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}

	terminal := waitForTerminalEvent(t, ch)
	if terminal.Type != agent.EventTurnFailed {
		t.Fatalf("expected terminal event turn_failed, got %q", terminal.Type)
	}
	if terminal.StopReason != "invalid_tool_input" {
		t.Fatalf("expected stop_reason invalid_tool_input, got %q", terminal.StopReason)
	}
}

func TestRunner_EmitsToolInputDeltaToEmitter(t *testing.T) {
	m := &mockModel{
		id: "mock",
		events: []models.StreamEvent{
			models.ToolUseStartEvent{ID: "tu-1", Name: "echo"},
			models.ToolInputDeltaEvent{ID: "tu-1", Delta: `{"message":"ok"}`},
			models.MessageDeltaEvent{StopReason: "tool_use", OutputTokens: 1},
		},
	}
	emitter := &captureEmitter{}
	reg := tools.NewRegistry()
	if err := reg.Register(tools.ToolDefinition{
		Name:             "echo",
		Version:          "1",
		SideEffect:       tools.SideEffectNone,
		ApprovalRequired: false,
		InputSchema:      map[string]tools.FieldSchema{"message": {Type: "string"}},
		OutputSchema:     map[string]tools.FieldSchema{"message": {Type: "string"}},
		Limits:           tools.ExecutionLimits{TimeoutSeconds: 5, MaxOutputBytes: 1024},
	}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	exec := tools.NewExecutor(reg, map[string]tools.ToolHandler{
		"echo": func(_ context.Context, input map[string]interface{}) (map[string]interface{}, error) {
			return input, nil
		},
	})

	cfg := agent.DefaultConfig("tier1")
	cfg.MaxDepth = 1
	runner := agent.NewRunner(m, reg, exec, noopGate{}, emitter, cfg)

	ch, err := runner.RunTurn(context.Background(), agent.AgentTurn{ID: "t-tool-input", SessionID: "s1", Message: "emit input delta", TrustTier: "tier1"})
	if err != nil {
		t.Fatalf("RunTurn error: %v", err)
	}
	_ = waitForTerminalEvent(t, ch)

	found := false
	for _, e := range emitter.Snapshot() {
		if e.Type == agent.EventToolInputDelta && e.ToolID == "tu-1" && e.Data == `{"message":"ok"}` {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected emitter to receive tool_input_delta event")
	}
}

func waitForTerminalEvent(t *testing.T, ch <-chan agent.AgentEvent) agent.AgentEvent {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				t.Fatal("event channel closed before terminal event")
			}
			if evt.Type == agent.EventTurnCompleted || evt.Type == agent.EventTurnFailed {
				return evt
			}
		case <-timeout:
			t.Fatal("timed out waiting for terminal event")
		}
	}
}
