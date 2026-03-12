package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

type failingLSPBroker struct {
	err error
}

func (f failingLSPBroker) Diagnostics(context.Context, string, string) ([]lspDiagnostic, error) {
	return nil, f.err
}

func (f failingLSPBroker) Symbols(context.Context, string, string) ([]lspSymbol, error) {
	return nil, f.err
}

func (f failingLSPBroker) Hover(context.Context, string, lspPositionRequest) (lspHoverResponse, error) {
	return lspHoverResponse{}, f.err
}

func (f failingLSPBroker) Definition(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return nil, f.err
}

func (f failingLSPBroker) References(context.Context, string, lspPositionRequest) ([]lspLocation, error) {
	return nil, f.err
}

func (f failingLSPBroker) Completions(context.Context, string, lspPositionRequest) ([]lspCompletionItem, error) {
	return nil, f.err
}

func TestLSPDiagnosticsRoute_ValidatesRequiredParams(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	server := &Server{sessionMgr: session.NewManager(eventStore), lspBroker: NoopLSPBroker{}, httpServer: &http.Server{}}
	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/diagnostics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLSPDiagnosticsRoute_ReturnsNotFoundForUnknownSession(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	server := &Server{sessionMgr: session.NewManager(eventStore), lspBroker: NoopLSPBroker{}, httpServer: &http.Server{}}
	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/diagnostics?session_id=s-missing&path=main.go", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLSPHoverRoute_ReturnsNormalizedPayload(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{sessionMgr: sessionMgr, lspBroker: NoopLSPBroker{}, eventStore: eventStore, httpServer: &http.Server{}}
	mux := http.NewServeMux()
	server.registerRoutes(mux)

	body := bytes.NewBufferString(`{"session_id":"` + created.ID + `","path":"main.go","line":1,"col":1}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/lsp/hover", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	events, err := eventStore.QueryBySession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	foundLSP := false
	for _, event := range events {
		if event.EventType == audit.EventLSPRequest {
			foundLSP = true
			break
		}
	}
	if !foundLSP {
		t.Fatal("expected lsp_request audit event")
	}
}

func TestLSPDiagnosticsRoute_UsesTypedLSPErrorEnvelope(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() { _ = eventStore.Close() })

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{
		sessionMgr: sessionMgr,
		lspBroker:  failingLSPBroker{err: newLSPError(LSPErrorServerUnavailable, http.StatusServiceUnavailable, false, "missing gopls", nil)},
		httpServer: &http.Server{},
		metrics:    newRuntimeMetrics(time.Now().UTC()),
	}
	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/diagnostics?session_id="+created.ID+"&path=main.go", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var envelope ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to decode envelope: %v", err)
	}
	if envelope.Type != string(LSPErrorServerUnavailable) {
		t.Fatalf("expected typed lsp error, got %q", envelope.Type)
	}
}
