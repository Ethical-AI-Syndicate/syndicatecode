package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

type lspRange struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

type lspDiagnostic struct {
	Path     string   `json:"path"`
	Code     string   `json:"code,omitempty"`
	Severity string   `json:"severity"`
	Message  string   `json:"message"`
	Range    lspRange `json:"range"`
}

type lspSymbol struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Path  string   `json:"path"`
	Range lspRange `json:"range"`
}

type lspPositionRequest struct {
	SessionID string `json:"session_id"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Col       int    `json:"col"`
}

type lspHoverResponse struct {
	Contents string   `json:"contents"`
	Range    lspRange `json:"range"`
}

type lspLocation struct {
	Path  string   `json:"path"`
	Range lspRange `json:"range"`
}

type lspCompletionItem struct {
	Label         string `json:"label"`
	Kind          string `json:"kind,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}

type LSPErrorCode string

const (
	LSPErrorServerUnavailable LSPErrorCode = "lsp_server_unavailable"
	LSPErrorBackendUnhealthy  LSPErrorCode = "lsp_backend_unhealthy"
	LSPErrorRequestTimeout    LSPErrorCode = "lsp_request_timeout"
)

type LSPError struct {
	Code      LSPErrorCode
	Status    int
	Reason    string
	Retryable bool
	Details   map[string]string
}

func (e *LSPError) Error() string {
	if e == nil {
		return ""
	}
	return e.Reason
}

func newLSPError(code LSPErrorCode, status int, retryable bool, reason string, details map[string]string) *LSPError {
	return &LSPError{Code: code, Status: status, Reason: reason, Retryable: retryable, Details: details}
}

func asLSPError(err error) (*LSPError, bool) {
	if err == nil {
		return nil, false
	}
	var lspErr *LSPError
	if errors.As(err, &lspErr) {
		return lspErr, true
	}
	return nil, false
}

type LSPBroker interface {
	Diagnostics(ctx context.Context, repoPath, path string) ([]lspDiagnostic, error)
	Symbols(ctx context.Context, repoPath, path string) ([]lspSymbol, error)
	Hover(ctx context.Context, repoPath string, req lspPositionRequest) (lspHoverResponse, error)
	Definition(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspLocation, error)
	References(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspLocation, error)
	Completions(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspCompletionItem, error)
}

type NoopLSPBroker struct{}

func (NoopLSPBroker) Diagnostics(context.Context, string, string) ([]lspDiagnostic, error) {
	return []lspDiagnostic{}, nil
}

func (NoopLSPBroker) Symbols(context.Context, string, string) ([]lspSymbol, error) {
	return []lspSymbol{}, nil
}

func (NoopLSPBroker) Hover(_ context.Context, _ string, _ lspPositionRequest) (lspHoverResponse, error) {
	return lspHoverResponse{Contents: "", Range: lspRange{}}, nil
}

func (NoopLSPBroker) Definition(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return []lspLocation{}, nil
}

func (NoopLSPBroker) References(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return []lspLocation{}, nil
}

func (NoopLSPBroker) Completions(context.Context, string, lspPositionRequest) ([]lspCompletionItem, error) {
	return []lspCompletionItem{}, nil
}

type ManagedLSPBroker struct {
	delegate LSPBroker
	process  *lspProcessSupervisor

	mu            sync.RWMutex
	cachedDiag    map[string][]lspDiagnostic
	cachedSymbols map[string][]lspSymbol
}

func NewManagedLSPBroker(delegate LSPBroker, process *lspProcessSupervisor) *ManagedLSPBroker {
	if delegate == nil {
		delegate = NoopLSPBroker{}
	}
	if process == nil {
		process = newLSPProcessSupervisor(nil, nil)
	}
	return &ManagedLSPBroker{
		delegate:      delegate,
		process:       process,
		cachedDiag:    make(map[string][]lspDiagnostic),
		cachedSymbols: make(map[string][]lspSymbol),
	}
}

func (b *ManagedLSPBroker) Diagnostics(ctx context.Context, repoPath, path string) ([]lspDiagnostic, error) {
	if err := b.ensureBackend(ctx, repoPath, path); err != nil {
		return nil, err
	}
	items, err := b.delegate.Diagnostics(ctx, repoPath, path)
	key := cacheKey(repoPath, path)
	if err != nil {
		if lspErr, ok := asLSPError(err); ok && lspErr.Code == LSPErrorRequestTimeout {
			if cached, found := b.loadCachedDiagnostics(key); found {
				return cached, nil
			}
		}
		return nil, err
	}
	b.storeCachedDiagnostics(key, items)
	return items, nil
}

func (b *ManagedLSPBroker) Symbols(ctx context.Context, repoPath, path string) ([]lspSymbol, error) {
	if err := b.ensureBackend(ctx, repoPath, path); err != nil {
		return nil, err
	}
	items, err := b.delegate.Symbols(ctx, repoPath, path)
	key := cacheKey(repoPath, path)
	if err != nil {
		if lspErr, ok := asLSPError(err); ok && lspErr.Code == LSPErrorRequestTimeout {
			if cached, found := b.loadCachedSymbols(key); found {
				return cached, nil
			}
		}
		return nil, err
	}
	b.storeCachedSymbols(key, items)
	return items, nil
}

func (b *ManagedLSPBroker) Hover(ctx context.Context, repoPath string, req lspPositionRequest) (lspHoverResponse, error) {
	if err := b.ensureBackend(ctx, repoPath, req.Path); err != nil {
		return lspHoverResponse{}, err
	}
	return b.delegate.Hover(ctx, repoPath, req)
}

func (b *ManagedLSPBroker) Definition(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspLocation, error) {
	if err := b.ensureBackend(ctx, repoPath, req.Path); err != nil {
		return nil, err
	}
	return b.delegate.Definition(ctx, repoPath, req)
}

func (b *ManagedLSPBroker) References(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspLocation, error) {
	if err := b.ensureBackend(ctx, repoPath, req.Path); err != nil {
		return nil, err
	}
	return b.delegate.References(ctx, repoPath, req)
}

func (b *ManagedLSPBroker) Completions(ctx context.Context, repoPath string, req lspPositionRequest) ([]lspCompletionItem, error) {
	if err := b.ensureBackend(ctx, repoPath, req.Path); err != nil {
		return nil, err
	}
	return b.delegate.Completions(ctx, repoPath, req)
}

func (b *ManagedLSPBroker) ensureBackend(ctx context.Context, repoPath, path string) error {
	detected := detectLanguage(repoPath, path)
	if b.process == nil {
		return nil
	}
	return b.process.EnsureStarted(ctx, detected)
}

func (b *ManagedLSPBroker) loadCachedDiagnostics(key string) ([]lspDiagnostic, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items, ok := b.cachedDiag[key]
	if !ok {
		return nil, false
	}
	return append([]lspDiagnostic(nil), items...), true
}

func (b *ManagedLSPBroker) storeCachedDiagnostics(key string, items []lspDiagnostic) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cachedDiag[key] = append([]lspDiagnostic(nil), items...)
}

func (b *ManagedLSPBroker) loadCachedSymbols(key string) ([]lspSymbol, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	items, ok := b.cachedSymbols[key]
	if !ok {
		return nil, false
	}
	return append([]lspSymbol(nil), items...), true
}

func (b *ManagedLSPBroker) storeCachedSymbols(key string, items []lspSymbol) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cachedSymbols[key] = append([]lspSymbol(nil), items...)
}

func cacheKey(repoPath, path string) string {
	return repoPath + "\x00" + path
}

func repoPathForSession(ctx context.Context, mgr sessionReader, sessionID string) (string, error) {
	if mgr == nil {
		return "", fmt.Errorf("session manager unavailable")
	}
	sess, err := mgr.Get(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if sess == nil {
		return "", fmt.Errorf("session repo path unavailable")
	}
	if sess.RepoPath == "" {
		if lister, ok := mgr.(sessionLister); ok {
			items, listErr := lister.List(ctx)
			if listErr == nil {
				for _, item := range items {
					if item != nil && item.ID == sessionID && item.RepoPath != "" {
						return item.RepoPath, nil
					}
				}
			}
		}
		return "", fmt.Errorf("session repo path unavailable")
	}
	return sess.RepoPath, nil
}

type sessionReader interface {
	Get(ctx context.Context, id string) (*session.Session, error)
}

type sessionLister interface {
	List(ctx context.Context) ([]*session.Session, error)
}
