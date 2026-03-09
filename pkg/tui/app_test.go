package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type mockAPI struct {
	sessions      []Session
	approvals     []Approval
	contextByTurn map[string][]ContextFragment
	decisions     []decisionCall
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
	s := &Session{ID: "s-new", RepoPath: req.RepoPath, TrustTier: req.TrustTier}
	return s, nil
}

func (m *mockAPI) CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error) {
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
