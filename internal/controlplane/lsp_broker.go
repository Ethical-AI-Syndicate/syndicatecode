package controlplane

import (
	"context"
	"fmt"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	api "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/pkg/api"
)

type LSPBroker interface {
	Diagnostics(ctx context.Context, repoPath, path string) ([]api.LSPDiagnostic, error)
	Symbols(ctx context.Context, repoPath, path string) ([]api.LSPSymbol, error)
	Hover(ctx context.Context, repoPath string, req api.LSPPositionRequest) (api.LSPHoverResponse, error)
	Definition(ctx context.Context, repoPath string, req api.LSPPositionRequest) ([]api.LSPLocation, error)
	References(ctx context.Context, repoPath string, req api.LSPPositionRequest) ([]api.LSPLocation, error)
	Completions(ctx context.Context, repoPath string, req api.LSPPositionRequest) ([]api.LSPCompletionItem, error)
}

type NoopLSPBroker struct{}

func (NoopLSPBroker) Diagnostics(context.Context, string, string) ([]api.LSPDiagnostic, error) {
	return []api.LSPDiagnostic{}, nil
}

func (NoopLSPBroker) Symbols(context.Context, string, string) ([]api.LSPSymbol, error) {
	return []api.LSPSymbol{}, nil
}

func (NoopLSPBroker) Hover(_ context.Context, _ string, _ api.LSPPositionRequest) (api.LSPHoverResponse, error) {
	return api.LSPHoverResponse{Contents: "", Range: api.LSPRange{}}, nil
}

func (NoopLSPBroker) Definition(context.Context, string, api.LSPPositionRequest) ([]api.LSPLocation, error) {
	return []api.LSPLocation{}, nil
}

func (NoopLSPBroker) References(context.Context, string, api.LSPPositionRequest) ([]api.LSPLocation, error) {
	return []api.LSPLocation{}, nil
}

func (NoopLSPBroker) Completions(context.Context, string, api.LSPPositionRequest) ([]api.LSPCompletionItem, error) {
	return []api.LSPCompletionItem{}, nil
}

func repoPathForSession(ctx context.Context, mgr sessionReader, sessionID string) (string, error) {
	if mgr == nil {
		return "", fmt.Errorf("session manager unavailable")
	}
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if sess == nil || sess.RepoPath == "" {
		return "", fmt.Errorf("session repo path unavailable")
	}
	return sess.RepoPath, nil
}

type sessionReader interface {
	Get(ctx context.Context, id string) (*session.Session, error)
}
