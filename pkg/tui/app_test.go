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
	diagnostics   []LSPDiagnostic
	symbols       []LSPSymbol
	hover         *LSPHoverResponse
	definitions   []LSPLocation
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

func (m *mockAPI) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	return nil, nil
}

func (m *mockAPI) GetHealth(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "ok"}, nil
}

func (m *mockAPI) GetReadiness(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{"status": "ready"}, nil
}

func (m *mockAPI) GetMetrics(ctx context.Context) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (m *mockAPI) GetPolicyRoute(ctx context.Context, trustTier, sensitivity, task string) (PolicyDocument, error) {
	return m.policy, nil
}

func (m *mockAPI) GetEventTypes(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	for _, events := range m.replayBySess {
		for _, ev := range events {
			seen[ev.EventType] = true
		}
	}
	types := make([]string, 0, len(seen))
	for et := range seen {
		types = append(types, et)
	}
	return types, nil
}

func (m *mockAPI) GetDiagnostics(ctx context.Context, sessionID, path string) ([]LSPDiagnostic, error) {
	_ = ctx
	_ = sessionID
	_ = path
	return m.diagnostics, nil
}

func (m *mockAPI) GetSymbols(ctx context.Context, sessionID, path string) ([]LSPSymbol, error) {
	_ = ctx
	_ = sessionID
	_ = path
	return m.symbols, nil
}

func (m *mockAPI) GetHover(ctx context.Context, req LSPPositionRequest) (*LSPHoverResponse, error) {
	_ = ctx
	_ = req
	return m.hover, nil
}

func (m *mockAPI) GetDefinition(ctx context.Context, req LSPPositionRequest) ([]LSPLocation, error) {
	_ = ctx
	_ = req
	return m.definitions, nil
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
		"diff-rich": {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/events"},
		"diag":      {Method: "GET", PathTemplate: "/api/v1/lsp/diagnostics"},
		"sym":       {Method: "GET", PathTemplate: "/api/v1/lsp/symbols"},
		"hover":     {Method: "POST", PathTemplate: "/api/v1/lsp/hover"},
		"def":       {Method: "POST", PathTemplate: "/api/v1/lsp/definition"},
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

func TestRenderFileMutation_RendersUnifiedDiffFromPatch_Bead_l3d_X_1(t *testing.T) {
	event := ReplayEvent{
		EventType: "file_mutation",
		Payload:   []byte(`{"path":"internal/foo/bar.go","type":"update","before_hash":"abc","after_hash":"def","patch":"--- a/internal/foo/bar.go\n+++ b/internal/foo/bar.go\n@@ -1,1 +1,1 @@\n-old\n+new"}`),
	}
	app := NewApp(nil, nil, nil)
	out := app.renderFileMutation(event)
	if !strings.Contains(out, "~ internal/foo/bar.go (update)") {
		t.Errorf("expected mutation header in output, got %q", out)
	}
	if !strings.Contains(out, "--- a/internal/foo/bar.go") || !strings.Contains(out, "+++ b/internal/foo/bar.go") {
		t.Errorf("expected unified diff file headers, got %q", out)
	}
	if !strings.Contains(out, "@@ -1,1 +1,1 @@") {
		t.Errorf("expected unified diff hunk header, got %q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI colorized diff output, got %q", out)
	}
}

func TestRenderFileMutation_RendersUnifiedDiffFromHunks_Bead_l3d_X_1(t *testing.T) {
	event := ReplayEvent{
		EventType: "file_mutation",
		Payload:   []byte(`{"path":"internal/foo/bar.go","type":"update","hunks":[{"old_start":10,"old_lines":2,"new_start":10,"new_lines":2,"lines":[" context","-old","+new"]}]}`),
	}
	app := NewApp(nil, nil, nil)
	out := app.renderFileMutation(event)

	if !strings.Contains(out, "--- a/internal/foo/bar.go") || !strings.Contains(out, "+++ b/internal/foo/bar.go") {
		t.Errorf("expected unified diff file headers, got %q", out)
	}
	if !strings.Contains(out, "@@ -10,2 +10,2 @@") {
		t.Errorf("expected unified diff hunk header, got %q", out)
	}
	if !strings.Contains(out, "-old") || !strings.Contains(out, "+new") {
		t.Errorf("expected hunk lines in output, got %q", out)
	}
	if strings.Contains(out, "before:") || strings.Contains(out, "after:") {
		t.Errorf("expected unified diff path, got fallback output %q", out)
	}
	if !strings.Contains(out, "~ internal/foo/bar.go (update)") {
		t.Errorf("expected mutation header in output, got %q", out)
	}
	if strings.Count(out, "@@") != 2 {
		t.Errorf("expected single hunk marker pair in output, got %q", out)
	}
}

func TestRenderFileMutation_FallsBackToStructuredSummaryWhenPatchMissing_Bead_l3d_X_1(t *testing.T) {
	event := ReplayEvent{
		EventType: "file_mutation",
		Payload:   []byte(`{"path":"internal/foo/bar.go","type":"update","before_hash":"abc","after_hash":"def"}`),
	}
	app := NewApp(nil, nil, nil)
	out := app.renderFileMutation(event)

	expected := "~ internal/foo/bar.go (update) before:abc after:def"
	if out != expected {
		t.Errorf("expected fallback summary %q, got %q", expected, out)
	}
}

func TestRenderFileMutation_FallsBackWhenHunksMalformed_Bead_l3d_X_1(t *testing.T) {
	event := ReplayEvent{
		EventType: "file_mutation",
		Payload:   []byte(`{"path":"internal/foo/bar.go","type":"update","before_hash":"abc","after_hash":"def","hunks":[{"old_start":"bad","old_lines":1,"new_start":1,"new_lines":1,"lines":["-old","+new"]}]}`),
	}
	app := NewApp(nil, nil, nil)
	out := app.renderFileMutation(event)

	expected := "~ internal/foo/bar.go (update) before:abc after:def"
	if out != expected {
		t.Fatalf("expected fallback summary %q, got %q", expected, out)
	}
	if strings.Contains(out, "--- a/") || strings.Contains(out, "@@") {
		t.Fatalf("expected no unified diff output, got %q", out)
	}
}

func TestJSONNumberToInt_RejectsNonIntegerNumbers_Bead_l3d_X_1(t *testing.T) {
	if _, ok := jsonNumberToInt(10.5); ok {
		t.Fatalf("expected non-integer float to be rejected")
	}
	if got, ok := jsonNumberToInt(10.0); !ok || got != 10 {
		t.Fatalf("expected integral float to be accepted, got (%d, %t)", got, ok)
	}
}

func TestApp_DiffRichCommand_RendersStructuredModel(t *testing.T) {
	api := &mockAPI{
		replayBySess: map[string][]ReplayEvent{
			"s-1": {
				{EventType: "file_mutation", Payload: []byte(`{"path":"a.go","type":"update","before_hash":"abc","after_hash":"def","hunks":[{"old_start":1,"old_lines":1,"new_start":1,"new_lines":1,"lines":["-old","+new"]}]}`)},
			},
		},
	}
	in := strings.NewReader("diff-rich s-1\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	content := out.String()
	if !strings.Contains(content, "> a.go (update)") || !strings.Contains(content, "@@ -1,1 +1,1 @@") {
		t.Fatalf("expected rich diff output, got %q", content)
	}
}

func TestApp_LSPCommands_RenderOutputs(t *testing.T) {
	api := &mockAPI{
		diagnostics: []LSPDiagnostic{{Path: "main.go", Severity: "error", Message: "undefined", Range: LSPRange{StartLine: 2, StartCol: 3}}},
		symbols:     []LSPSymbol{{Kind: "function", Name: "main", Path: "main.go", Range: LSPRange{StartLine: 1}}},
		hover:       &LSPHoverResponse{Contents: "func main()"},
		definitions: []LSPLocation{{Path: "main.go", Range: LSPRange{StartLine: 10, StartCol: 1}}},
	}
	in := strings.NewReader("diag s-1 main.go\nsym s-1 main.go\nhover s-1 main.go 1 1\ndef s-1 main.go 1 1\nquit\n")
	out := &bytes.Buffer{}
	app := NewApp(api, in, out)

	if err := app.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	content := out.String()
	if !strings.Contains(content, "main.go:2:3 [error] undefined") {
		t.Fatalf("expected diagnostics output, got %q", content)
	}
	if !strings.Contains(content, "function main main.go:1") {
		t.Fatalf("expected symbols output, got %q", content)
	}
	if !strings.Contains(content, "func main()") {
		t.Fatalf("expected hover output, got %q", content)
	}
	if !strings.Contains(content, "main.go:10:1") {
		t.Fatalf("expected definition output, got %q", content)
	}
}
