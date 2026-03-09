package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

type API interface {
	ListSessions(ctx context.Context) ([]Session, error)
	CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error)
	CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error)
	ListApprovals(ctx context.Context) ([]Approval, error)
	DecideApproval(ctx context.Context, approvalID string, req DecideApprovalRequest) (*Approval, error)
	GetTurnContext(ctx context.Context, sessionID, turnID string) ([]ContextFragment, error)
}

type App struct {
	api    API
	input  io.Reader
	output io.Writer
}

func NewApp(api API, input io.Reader, output io.Writer) *App {
	return &App{api: api, input: input, output: output}
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
	case "quit", "exit":
		return true, nil
	case "sessions":
		return false, a.handleSessions(ctx)
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
	default:
		return false, a.writeln("unknown command")
	}
}

func (a *App) printHelp() error {
	lines := []string{
		"Commands:",
		"  sessions",
		"  new-session <repo_path> <trust_tier>",
		"  turn <session_id> <message>",
		"  approvals",
		"  approve <approval_id>",
		"  deny <approval_id> [reason]",
		"  context <turn_id>",
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
	approval, err := a.api.DecideApproval(ctx, approvalID, DecideApprovalRequest{Decision: decision, Reason: reason})
	if err != nil {
		return err
	}
	return a.writef("%s -> %s\n", approval.ID, approval.State)
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

func (a *App) writef(format string, args ...interface{}) error {
	_, err := fmt.Fprintf(a.output, format, args...)
	return err
}

func (a *App) writeln(line string) error {
	_, err := fmt.Fprintln(a.output, line)
	return err
}
