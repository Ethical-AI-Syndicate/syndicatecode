package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

type API interface {
	ListSessions(ctx context.Context) ([]Session, error)
	CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error)
	CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	DecideApproval(ctx context.Context, approvalID string, req DecideApprovalRequest) (*Approval, error)
	GetTurnContext(ctx context.Context, sessionID, turnID string) ([]ContextFragment, error)
	GetPolicy(ctx context.Context) (PolicyDocument, error)
	ListSessionEvents(ctx context.Context, sessionID, eventType string) ([]ReplayEvent, error)
	GetEventTypes(ctx context.Context) ([]string, error)
	GetDiagnostics(ctx context.Context, sessionID, path string) ([]LSPDiagnostic, error)
	GetSymbols(ctx context.Context, sessionID, path string) ([]LSPSymbol, error)
	GetHover(ctx context.Context, req LSPPositionRequest) (*LSPHoverResponse, error)
	GetDefinition(ctx context.Context, req LSPPositionRequest) ([]LSPLocation, error)
}

type App struct {
	api    API
	input  io.Reader
	output io.Writer
}

type endpointBinding struct {
	Method       string
	PathTemplate string
}

func NewApp(api API, input io.Reader, output io.Writer) *App {
	return &App{api: api, input: input, output: output}
}

func commandMappings() map[string]endpointBinding {
	return map[string]endpointBinding{
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
}

func (a *App) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(a.input)
	if err := a.writeln("SyndicateCode TUI"); err != nil {
		return err
	}
	if err := a.writeln("Type 'help' for commands"); err != nil {
		return err
	}

	for {
		if _, err := fmt.Fprint(a.output, "> "); err != nil {
			return err
		}
		if !scanner.Scan() {
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		args := strings.Fields(line)
		shouldExit, err := a.executeCommand(ctx, args)
		if err != nil {
			if writeErr := a.writef("error: %v\n", err); writeErr != nil {
				return writeErr
			}
		}
		if shouldExit {
			return nil
		}
	}
}

//nolint:gocyclo // Command dispatch intentionally centralizes CLI routing.
func (a *App) executeCommand(ctx context.Context, args []string) (bool, error) {
	cmd := args[0]

	switch cmd {
	case "help":
		return false, a.printHelp()
	case "quit", "exit", "stop":
		return true, nil
	case "sessions":
		return false, a.handleSessions(ctx)
	case "start":
		if len(args) < 3 {
			return false, a.writeln("usage: start <repo_path> <trust_tier>")
		}
		return false, a.handleNewSession(ctx, args[1], args[2])
	case "ask":
		if len(args) < 3 {
			return false, a.writeln("usage: ask <session_id> <message>")
		}
		message := strings.TrimSpace(strings.Join(args[2:], " "))
		return false, a.handleTurn(ctx, args[1], message)
	case "new-session":
		if len(args) < 3 {
			return false, a.writeln("usage: new-session <repo_path> <trust_tier>")
		}
		return false, a.handleNewSession(ctx, args[1], args[2])
	case "turn":
		if len(args) < 3 {
			return false, a.writeln("usage: turn <session_id> <message>")
		}
		message := strings.TrimSpace(strings.Join(args[2:], " "))
		return false, a.handleTurn(ctx, args[1], message)
	case "approvals":
		return false, a.handleApprovals(ctx)
	case "approve":
		if len(args) < 2 {
			return false, a.writeln("usage: approve <approval_id>")
		}
		return false, a.handleDecision(ctx, args[1], "approve", "")
	case "deny":
		if len(args) < 2 {
			return false, a.writeln("usage: deny <approval_id> [reason]")
		}
		reason := ""
		if len(args) > 2 {
			reason = strings.Join(args[2:], " ")
		}
		return false, a.handleDecision(ctx, args[1], "deny", reason)
	case "context":
		if len(args) < 3 {
			return false, a.writeln("usage: context <session_id> <turn_id>")
		}
		return false, a.handleContext(ctx, args[1], args[2])
	case "policy":
		return false, a.handlePolicy(ctx)
	case "replay":
		if len(args) < 2 {
			return false, a.writeln("usage: replay <session_id> [event_type]")
		}
		eventType := ""
		if len(args) > 2 {
			eventType = args[2]
		}
		return false, a.handleReplay(ctx, args[1], eventType)
	case "diff":
		if len(args) < 2 {
			return false, a.writeln("usage: diff <session_id> [event_type]")
		}
		eventType := ""
		if len(args) > 2 {
			eventType = args[2]
		}
		return false, a.handleReplay(ctx, args[1], eventType)
	case "diff-rich":
		if len(args) < 2 {
			return false, a.writeln("usage: diff-rich <session_id> [event_type]")
		}
		eventType := ""
		if len(args) > 2 {
			eventType = args[2]
		}
		return false, a.handleDiffRich(ctx, args[1], eventType)
	case "diag":
		if len(args) < 3 {
			return false, a.writeln("usage: diag <session_id> <path>")
		}
		return false, a.handleDiagnostics(ctx, args[1], args[2])
	case "sym":
		if len(args) < 3 {
			return false, a.writeln("usage: sym <session_id> <path>")
		}
		return false, a.handleSymbols(ctx, args[1], args[2])
	case "hover":
		if len(args) < 5 {
			return false, a.writeln("usage: hover <session_id> <path> <line> <col>")
		}
		return false, a.handleHover(ctx, args[1], args[2], args[3], args[4])
	case "def":
		if len(args) < 5 {
			return false, a.writeln("usage: def <session_id> <path> <line> <col>")
		}
		return false, a.handleDefinition(ctx, args[1], args[2], args[3], args[4])
	default:
		return false, a.writeln("unknown command")
	}
}

func (a *App) printHelp() error {
	lines := []string{
		"Commands:",
		"  sessions",
		"  start <repo_path> <trust_tier>",
		"  ask <session_id> <message>",
		"  diff <session_id> [event_type]",
		"  diff-rich <session_id> [event_type]",
		"  diag <session_id> <path>",
		"  sym <session_id> <path>",
		"  hover <session_id> <path> <line> <col>",
		"  def <session_id> <path> <line> <col>",
		"  new-session <repo_path> <trust_tier>",
		"  turn <session_id> <message>",
		"  approvals",
		"  approve <approval_id>",
		"  deny <approval_id> [reason]",
		"  context <session_id> <turn_id>",
		"  policy",
		"  replay <session_id> [event_type]",
		"  stop",
		"  quit",
	}
	for _, line := range lines {
		if err := a.writeln(line); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleSessions(ctx context.Context) error {
	sessions, err := a.api.ListSessions(ctx)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if err := a.writef("%s %s %s %s\n", s.ID, s.RepoPath, s.TrustTier, s.Status); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleNewSession(ctx context.Context, repoPath, trustTier string) error {
	session, err := a.api.CreateSession(ctx, CreateSessionRequest{RepoPath: repoPath, TrustTier: trustTier})
	if err != nil {
		return err
	}
	return a.writef("created session: %s\n", session.ID)
}

func (a *App) handleTurn(ctx context.Context, sessionID, message string) error {
	turn, err := a.api.CreateTurn(ctx, sessionID, CreateTurnRequest{Message: message})
	if err != nil {
		return err
	}
	return a.writef("created turn: %s\n", turn.ID)
}

func (a *App) handleApprovals(ctx context.Context) error {
	approvals, err := a.api.ListApprovals(ctx)
	if err != nil {
		return err
	}
	for _, approval := range approvals {
		if err := a.writef("%s %s %s\n", approval.ID, approval.ToolName, approval.State); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleDecision(ctx context.Context, approvalID, decision, reason string) error {
	approval, err := a.findPendingApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if approval.ExpiresAt != "" {
		expiresAt, parseErr := time.Parse(time.RFC3339, approval.ExpiresAt)
		if parseErr == nil && time.Now().UTC().After(expiresAt) {
			return fmt.Errorf("approval %s expired", approvalID)
		}
	}

	decided, err := a.api.DecideApproval(ctx, approvalID, DecideApprovalRequest{Decision: decision, Reason: reason})
	if err != nil {
		return err
	}
	return a.writef("%s -> %s\n", decided.ID, decided.State)
}

func (a *App) findPendingApproval(ctx context.Context, approvalID string) (*Approval, error) {
	approvals, err := a.api.ListApprovals(ctx)
	if err != nil {
		return nil, err
	}
	for _, approval := range approvals {
		if approval.ID != approvalID {
			continue
		}
		if approval.State != "pending" {
			return nil, fmt.Errorf("approval %s is in state %s", approvalID, approval.State)
		}
		copy := approval
		return &copy, nil
	}
	return nil, errors.New("approval not found")
}

func (a *App) handleContext(ctx context.Context, sessionID, turnID string) error {
	fragments, err := a.api.GetTurnContext(ctx, sessionID, turnID)
	if err != nil {
		return err
	}
	for _, f := range fragments {
		if err := a.writef("%s %s tokens=%d truncated=%t\n", f.SourceType, f.SourceRef, f.TokenCount, f.Truncated); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handlePolicy(ctx context.Context) error {
	policy, err := a.api.GetPolicy(ctx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	return a.writef("%s\n", string(data))
}

func (a *App) handleReplay(ctx context.Context, sessionID, eventType string) error {
	if eventType != "" {
		eventTypes, err := a.api.GetEventTypes(ctx)
		if err == nil && !containsString(eventTypes, eventType) {
			return fmt.Errorf("unsupported event_type %q", eventType)
		}
	}

	events, err := a.api.ListSessionEvents(ctx, sessionID, eventType)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if ev.EventType == "file_mutation" {
			if err := a.writeln(a.renderFileMutation(ev)); err != nil {
				return err
			}
			continue
		}
		if err := a.writef("%s %s %s\n", ev.Timestamp, ev.EventType, ev.Actor); err != nil {
			return err
		}
	}
	return nil
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func (a *App) handleDiffRich(ctx context.Context, sessionID, eventType string) error {
	events, err := a.api.ListSessionEvents(ctx, sessionID, eventType)
	if err != nil {
		return err
	}
	model, err := BuildRichDiffModel(events)
	if err != nil {
		return err
	}
	return a.writeln(model.Render())
}

func (a *App) handleDiagnostics(ctx context.Context, sessionID, path string) error {
	diagnostics, err := a.api.GetDiagnostics(ctx, sessionID, path)
	if err != nil {
		return err
	}
	if len(diagnostics) == 0 {
		return a.writeln("(no diagnostics)")
	}
	for _, diag := range diagnostics {
		if err := a.writef("%s:%d:%d [%s] %s\n", diag.Path, diag.Range.StartLine, diag.Range.StartCol, diag.Severity, diag.Message); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleSymbols(ctx context.Context, sessionID, path string) error {
	symbols, err := a.api.GetSymbols(ctx, sessionID, path)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return a.writeln("(no symbols)")
	}
	for _, symbol := range symbols {
		if err := a.writef("%s %s %s:%d\n", symbol.Kind, symbol.Name, symbol.Path, symbol.Range.StartLine); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleHover(ctx context.Context, sessionID, path, lineRaw, colRaw string) error {
	line, col, err := parseLineCol(lineRaw, colRaw)
	if err != nil {
		return err
	}
	hover, err := a.api.GetHover(ctx, LSPPositionRequest{SessionID: sessionID, Path: path, Line: line, Col: col})
	if err != nil {
		return err
	}
	if hover == nil || strings.TrimSpace(hover.Contents) == "" {
		return a.writeln("(no hover)")
	}
	return a.writef("%s\n", hover.Contents)
}

func (a *App) handleDefinition(ctx context.Context, sessionID, path, lineRaw, colRaw string) error {
	line, col, err := parseLineCol(lineRaw, colRaw)
	if err != nil {
		return err
	}
	locations, err := a.api.GetDefinition(ctx, LSPPositionRequest{SessionID: sessionID, Path: path, Line: line, Col: col})
	if err != nil {
		return err
	}
	if len(locations) == 0 {
		return a.writeln("(no definition)")
	}
	for _, loc := range locations {
		if err := a.writef("%s:%d:%d\n", loc.Path, loc.Range.StartLine, loc.Range.StartCol); err != nil {
			return err
		}
	}
	return nil
}

func parseLineCol(lineRaw, colRaw string) (int, int, error) {
	line, err := strconv.Atoi(lineRaw)
	if err != nil || line <= 0 {
		return 0, 0, fmt.Errorf("line must be a positive integer")
	}
	col, err := strconv.Atoi(colRaw)
	if err != nil || col <= 0 {
		return 0, 0, fmt.Errorf("col must be a positive integer")
	}
	return line, col, nil
}

func (a *App) renderFileMutation(event ReplayEvent) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return "[file_mutation] (unparseable payload)"
	}

	path, _ := payload["path"].(string)
	mutType, _ := payload["type"].(string)
	before, _ := payload["before_hash"].(string)
	after, _ := payload["after_hash"].(string)

	if diff := unifiedDiffFromPayload(payload, path); diff != "" {
		if path == "" && mutType == "" {
			return diff
		}
		return fmt.Sprintf("~ %s (%s)\n%s", path, mutType, diff)
	}

	if before == "" && after == "" {
		return fmt.Sprintf("~ %s (%s)", path, mutType)
	}
	return fmt.Sprintf("~ %s (%s) before:%s after:%s", path, mutType, before, after)
}

func unifiedDiffFromPayload(payload map[string]interface{}, path string) string {
	if patchText, ok := payload["patch"].(string); ok {
		trimmed := strings.TrimSpace(patchText)
		if trimmed != "" {
			return colorizeUnifiedDiff(trimmed)
		}
	}

	rawHunks, ok := payload["hunks"].([]interface{})
	if !ok || len(rawHunks) == 0 {
		return ""
	}

	filePath := path
	if filePath == "" {
		filePath = "file"
	}

	var b strings.Builder
	b.WriteString("--- a/")
	b.WriteString(filePath)
	b.WriteString("\n")
	b.WriteString("+++ b/")
	b.WriteString(filePath)

	emittedHunk := false

	for _, rawHunk := range rawHunks {
		hunk, ok := rawHunk.(map[string]interface{})
		if !ok {
			continue
		}
		oldStart, okOldStart := jsonNumberToInt(hunk["old_start"])
		oldLines, okOldLines := jsonNumberToInt(hunk["old_lines"])
		newStart, okNewStart := jsonNumberToInt(hunk["new_start"])
		newLines, okNewLines := jsonNumberToInt(hunk["new_lines"])
		if !okOldStart || !okOldLines || !okNewStart || !okNewLines {
			continue
		}
		emittedHunk = true

		b.WriteString("\n@@ -")
		b.WriteString(strconv.Itoa(oldStart))
		b.WriteString(",")
		b.WriteString(strconv.Itoa(oldLines))
		b.WriteString(" +")
		b.WriteString(strconv.Itoa(newStart))
		b.WriteString(",")
		b.WriteString(strconv.Itoa(newLines))
		b.WriteString(" @@")

		if lines, ok := hunk["lines"].([]interface{}); ok {
			for _, rawLine := range lines {
				line, ok := rawLine.(string)
				if !ok {
					continue
				}
				b.WriteString("\n")
				b.WriteString(line)
			}
		}
	}

	if !emittedHunk {
		return ""
	}

	return colorizeUnifiedDiff(b.String())
}

func colorizeUnifiedDiff(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return diff
	}
	const (
		ansiReset = "\x1b[0m"
		ansiRed   = "\x1b[31m"
		ansiGreen = "\x1b[32m"
		ansiCyan  = "\x1b[36m"
	)
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "@@"):
			lines[i] = ansiCyan + line + ansiReset
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			lines[i] = ansiGreen + line + ansiReset
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			lines[i] = ansiRed + line + ansiReset
		}
	}
	return strings.Join(lines, "\n")
}

func jsonNumberToInt(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case float64:
		if math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func (a *App) writef(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(a.output, format, args...)
	return err
}

func (a *App) writeln(line string) error {
	_, err := fmt.Fprintln(a.output, line)
	return err
}
