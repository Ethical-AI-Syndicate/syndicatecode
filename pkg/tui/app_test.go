package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type mockAPI struct {
	sessions      []Session
	approvals     []Approval
	contextByTurn map[string][]ContextFragment
	policy        PolicyDocument
	replayBySess  map[string][]ReplayEvent
	decisions     []decisionCall
	newSessions   int
	newTurns      int
}

type decisionCall struct {
	ID       string
	Decision string
	Reason   string
}

func (m *mockAPI) ListSessions(ctx context.Context) ([]Session, error) {
	return m.sessions, nil
}

func (m *mockAPI) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	m.newSessions++
	s := &Session{ID: "s-new", RepoPath: req.RepoPath, TrustTier: req.TrustTier}
	return s, nil
}

func (m *mockAPI) CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error) {
	m.newTurns++
	return &Turn{ID: "t-1", SessionID: sessionID, Message: req.Message}, nil
}

func (m *mockAPI) ListApprovals(ctx context.Context) ([]Approval, error) {
	return m.approvals, nil
}

func (m *mockAPI) DecideApproval(ctx context.Context, approvalID string, req DecideApprovalRequest) (*Approval, error) {
	m.decisions = append(m.decisions, decisionCall{ID: approvalID, Decision: req.Decision, Reason: req.Reason})
	return &Approval{ID: approvalID, State: "executed"}, nil
}

func (m *mockAPI) GetTurnContext(ctx context.Context, sessionID, turnID string) ([]ContextFragment, error) {
	key := sessionID + ":" + turnID
	return m.contextByTurn[key], nil
}

func (m *mockAPI) GetPolicy(ctx context.Context) (PolicyDocument, error) {
	return m.policy, nil
}

func (m *mockAPI) ListSessionEvents(ctx context.Context, sessionID, eventType string) ([]ReplayEvent, error) {
	events := m.replayBySess[sessionID]
	if eventType == "" {
		return events, nil
	}
	filtered := make([]ReplayEvent, 0, len(events))
	for _, ev := range events {
		if ev.EventType == eventType {
			filtered = append(filtered, ev)
		}
	}
	return filtered, nil
}

func TestApp_HelpAndQuit(t *testing.T) {
	in := strings.NewReader("help\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(&mockAPI{}, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	content := out.String()
	if !strings.Contains(content, "Commands:") {
		t.Fatalf("expected help output, got %q", content)
	}
}

func TestApp_ApprovalCommands(t *testing.T) {
	api := &mockAPI{
		approvals: []Approval{{ID: "apr-1", ToolName: "write_file", State: "pending"}},
	}
	in := strings.NewReader("approvals\napprove apr-1\ndeny apr-1 not-safe\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(api.decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(api.decisions))
	}
	if api.decisions[0].Decision != "approve" {
		t.Fatalf("expected first decision to be approve, got %s", api.decisions[0].Decision)
	}
	if api.decisions[1].Decision != "deny" || api.decisions[1].Reason != "not-safe" {
		t.Fatalf("unexpected deny decision: %+v", api.decisions[1])
	}
}

func TestApp_ContextCommand(t *testing.T) {
	api := &mockAPI{
		contextByTurn: map[string][]ContextFragment{
			"s-1:t-1": {{SourceType: "file", SourceRef: "main.go", TokenCount: 10}},
		},
	}
	in := strings.NewReader("context s-1 t-1\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if !strings.Contains(out.String(), "main.go") {
		t.Fatalf("expected context output, got %q", out.String())
	}
}

func TestApp_PolicyAndReplayCommands_Bead_l3d_15_4(t *testing.T) {
	api := &mockAPI{
		policy: PolicyDocument{
			"version": "1.0.0",
		},
		replayBySess: map[string][]ReplayEvent{
			"s-1": {
				{Timestamp: "2026-03-10T00:00:00Z", EventType: "session_started", Actor: "system"},
				{Timestamp: "2026-03-10T00:01:00Z", EventType: "mcp.call", Actor: "controlplane"},
			},
		},
	}
	in := strings.NewReader("policy\nreplay s-1 mcp.call\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	content := out.String()
	if !strings.Contains(content, `{"version":"1.0.0"}`) {
		t.Fatalf("expected policy json output, got %q", content)
	}
	if !strings.Contains(content, "2026-03-10T00:01:00Z mcp.call controlplane") {
		t.Fatalf("expected replay output for mcp.call, got %q", content)
	}
}

func TestApp_PrimaryCommandSurface_Bead_l3d_15_1(t *testing.T) {
	api := &mockAPI{
		replayBySess: map[string][]ReplayEvent{
			"s-new": {
				{Timestamp: "2026-03-10T00:02:00Z", EventType: "mcp.call", Actor: "controlplane"},
			},
		},
	}
	in := strings.NewReader("start /repo tier1\nask s-new hello world\ndiff s-new mcp.call\nstop\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	content := out.String()
	if !strings.Contains(content, "created session: s-new") {
		t.Fatalf("expected start command output, got %q", content)
	}
	if !strings.Contains(content, "created turn: t-1") {
		t.Fatalf("expected ask command output, got %q", content)
	}
	if !strings.Contains(content, "2026-03-10T00:02:00Z mcp.call controlplane") {
		t.Fatalf("expected diff replay output, got %q", content)
	}
}

func TestCommandMappings_Bead_l3d_15_2(t *testing.T) {
	mappings := commandMappings()

	required := map[string]endpointBinding{
		"start":     {Method: "POST", PathTemplate: "/api/v1/sessions"},
		"ask":       {Method: "POST", PathTemplate: "/api/v1/sessions/{session_id}/turns"},
		"approvals": {Method: "GET", PathTemplate: "/api/v1/approvals"},
		"approve":   {Method: "POST", PathTemplate: "/api/v1/approvals/{approval_id}"},
		"deny":      {Method: "POST", PathTemplate: "/api/v1/approvals/{approval_id}"},
		"context":   {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/turns/{turn_id}/context"},
		"policy":    {Method: "GET", PathTemplate: "/api/v1/policy"},
		"replay":    {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/events"},
		"diff":      {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/events"},
	}

	for command, expected := range required {
		actual, ok := mappings[command]
		if !ok {
			t.Fatalf("missing mapping for command %q", command)
		}
		if actual != expected {
			t.Fatalf("unexpected mapping for command %q: got %+v want %+v", command, actual, expected)
		}
	}
}

func TestApp_ApprovalCommandsRejectInvalidLifecycle_Bead_l3d_15_3(t *testing.T) {
	api := &mockAPI{
		approvals: []Approval{{ID: "apr-1", ToolName: "write_file", State: "executed", ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}},
	}
	in := strings.NewReader("approve apr-1\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(api.decisions) != 0 {
		t.Fatalf("expected no approval decisions to be submitted, got %d", len(api.decisions))
	}
	if !strings.Contains(out.String(), "approval apr-1 is in state executed") {
		t.Fatalf("expected lifecycle validation error, got %q", out.String())
	}
}

func TestApp_ApprovalCommandsRejectExpired_Bead_l3d_15_3(t *testing.T) {
	api := &mockAPI{
		approvals: []Approval{{ID: "apr-2", ToolName: "write_file", State: "pending", ExpiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}},
	}
	in := strings.NewReader("approve apr-2\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(api.decisions) != 0 {
		t.Fatalf("expected no approval decisions to be submitted, got %d", len(api.decisions))
	}
	if !strings.Contains(out.String(), "approval apr-2 expired") {
		t.Fatalf("expected expiry validation error, got %q", out.String())
	}
}

func TestApp_ReplayModeIsReadOnly_Bead_l3d_12_4(t *testing.T) {
	api := &mockAPI{
		replayBySess: map[string][]ReplayEvent{
			"s-1": {
				{Timestamp: "2026-03-10T00:00:00Z", EventType: "session_started", Actor: "system"},
				{Timestamp: "2026-03-10T00:01:00Z", EventType: "turn_completed", Actor: "system"},
			},
		},
	}
	in := strings.NewReader("replay s-1\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if api.newSessions != 0 || api.newTurns != 0 || len(api.decisions) != 0 {
		t.Fatalf("replay must be read-only; mutations observed sessions=%d turns=%d decisions=%d", api.newSessions, api.newTurns, len(api.decisions))
	}
	content := out.String()
	if !strings.Contains(content, "2026-03-10T00:00:00Z session_started system") || !strings.Contains(content, "2026-03-10T00:01:00Z turn_completed system") {
		t.Fatalf("expected ordered replay output, got %q", content)
	}
}
