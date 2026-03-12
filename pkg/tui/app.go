package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type API interface {
	ListSessions(ctx context.Context) ([]Session, error)
	CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error)
	CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error)
	ListTools(ctx context.Context) ([]ToolDefinition, error)
	GetHealth(ctx context.Context) (map[string]interface{}, error)
	GetReadiness(ctx context.Context) (map[string]interface{}, error)
	GetMetrics(ctx context.Context) (map[string]interface{}, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	DecideApproval(ctx context.Context, approvalID string, req DecideApprovalRequest) (*Approval, error)
	GetTurnContext(ctx context.Context, sessionID, turnID string) ([]ContextFragment, error)
	GetPolicy(ctx context.Context) (PolicyDocument, error)
	GetPolicyRoute(ctx context.Context, trustTier, sensitivity, task string) (PolicyDocument, error)
	GetEventTypes(ctx context.Context) ([]string, error)
	ListSessionEvents(ctx context.Context, sessionID, eventType string) ([]ReplayEvent, error)
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
		"start":        {Method: "POST", PathTemplate: "/api/v1/sessions"},
		"ask":          {Method: "POST", PathTemplate: "/api/v1/sessions/{session_id}/turns"},
		"tools":        {Method: "GET", PathTemplate: "/api/v1/tools"},
		"health":       {Method: "GET", PathTemplate: "/api/v1/health"},
		"readiness":    {Method: "GET", PathTemplate: "/api/v1/readiness"},
		"metrics":      {Method: "GET", PathTemplate: "/api/v1/metrics"},
		"approvals":    {Method: "GET", PathTemplate: "/api/v1/approvals"},
		"approve":      {Method: "POST", PathTemplate: "/api/v1/approvals/{approval_id}"},
		"deny":         {Method: "POST", PathTemplate: "/api/v1/approvals/{approval_id}"},
		"context":      {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/turns/{turn_id}/context"},
		"policy":       {Method: "GET", PathTemplate: "/api/v1/policy"},
		"policy-route": {Method: "GET", PathTemplate: "/api/v1/policy?trust_tier={trust_tier}&sensitivity={sensitivity}&task={task}"},
		"event-types":  {Method: "GET", PathTemplate: "/api/v1/events/types"},
		"replay":       {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/events"},
		"diff":         {Method: "GET", PathTemplate: "/api/v1/sessions/{session_id}/events"},
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

func (a *App) executeCommand(ctx context.Context, args []string) (bool, error) {
	cmd := args[0]

	switch cmd {
	case "help":
		return false, a.printHelp()
	case "quit", "exit", "stop":
		return true, nil
	case "sessions":
		return false, a.handleSessions(ctx)
	case "start", "new-session":
		if len(args) < 3 {
			return false, a.writeln("usage: start <repo_path> <trust_tier>")
		}
		return false, a.handleNewSession(ctx, args[1], args[2])
	case "ask", "turn":
		if len(args) < 3 {
			return false, a.writeln("usage: ask <session_id> <message>")
		}
		message := strings.TrimSpace(strings.Join(args[2:], " "))
		return false, a.handleTurn(ctx, args[1], message)
	case "approvals":
		return false, a.handleApprovals(ctx)
	case "tools":
		return false, a.handleTools(ctx)
	case "health":
		return false, a.handleHealth(ctx)
	case "readiness":
		return false, a.handleReadiness(ctx)
	case "metrics":
		return false, a.handleMetrics(ctx)
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
	case "policy-route":
		if len(args) < 4 {
			return false, a.writeln("usage: policy-route <trust_tier> <sensitivity> <task>")
		}
		return false, a.handlePolicyRoute(ctx, args[1], args[2], args[3])
	case "event-types":
		return false, a.handleEventTypes(ctx)
	case "replay", "diff":
		if len(args) < 2 {
			return false, a.writeln("usage: replay <session_id> [event_type]")
		}
		eventType := ""
		if len(args) > 2 {
			eventType = args[2]
		}
		return false, a.handleReplay(ctx, args[1], eventType)
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
		"  tools",
		"  health",
		"  readiness",
		"  metrics",
		"  diff <session_id> [event_type]",
		"  new-session <repo_path> <trust_tier>",
		"  turn <session_id> <message>",
		"  approvals",
		"  approve <approval_id>",
		"  deny <approval_id> [reason]",
		"  context <session_id> <turn_id>",
		"  policy",
		"  policy-route <trust_tier> <sensitivity> <task>",
		"  event-types",
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

func (a *App) handleTools(ctx context.Context) error {
	definitions, err := a.api.ListTools(ctx)
	if err != nil {
		return err
	}
	for _, definition := range definitions {
		if err := a.writef("%s %s side_effect=%s approval=%t\n", definition.Name, definition.Version, definition.SideEffect, definition.ApprovalRequired); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) handleHealth(ctx context.Context) error {
	status, err := a.api.GetHealth(ctx)
	if err != nil {
		return err
	}
	return a.writeJSON(status)
}

func (a *App) handleReadiness(ctx context.Context) error {
	status, err := a.api.GetReadiness(ctx)
	if err != nil {
		return err
	}
	return a.writeJSON(status)
}

func (a *App) handleMetrics(ctx context.Context) error {
	status, err := a.api.GetMetrics(ctx)
	if err != nil {
		return err
	}
	return a.writeJSON(status)
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

func (a *App) handlePolicyRoute(ctx context.Context, trustTier, sensitivity, task string) error {
	route, err := a.api.GetPolicyRoute(ctx, trustTier, sensitivity, task)
	if err != nil {
		return err
	}
	return a.writeJSON(route)
}

func (a *App) handleEventTypes(ctx context.Context) error {
	eventTypes, err := a.api.GetEventTypes(ctx)
	if err != nil {
		return err
	}
	for _, eventType := range eventTypes {
		if err := a.writef("%s\n", eventType); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) writeJSON(payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return a.writef("%s\n", string(data))
}

func (a *App) handleReplay(ctx context.Context, sessionID, eventType string) error {
	if eventType != "" {
		eventTypes, err := a.api.GetEventTypes(ctx)
		if err != nil {
			return err
		}
		if !containsString(eventTypes, eventType) {
			return fmt.Errorf("unsupported event_type %q", eventType)
		}
	}

	events, err := a.api.ListSessionEvents(ctx, sessionID, eventType)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := a.writef("%s %s %s\n", ev.Timestamp, ev.EventType, ev.Actor); err != nil {
			return err
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (a *App) writef(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(a.output, format, args...)
	return err
}

func (a *App) writeln(line string) error {
	_, err := fmt.Fprintln(a.output, line)
	return err
}
