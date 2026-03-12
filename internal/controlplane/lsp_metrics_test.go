package controlplane

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func TestLSPMetrics_RecordsRequestsAndErrors(t *testing.T) {
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
		lspBroker:  failingLSPBroker{err: newLSPError(LSPErrorServerUnavailable, http.StatusServiceUnavailable, false, "missing", nil)},
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

	hoverReq := httptest.NewRequest(http.MethodPost, "/api/v1/lsp/hover", bytes.NewBufferString(`{"session_id":"`+created.ID+`","path":"main.go","line":1,"col":1}`))
	hoverRec := httptest.NewRecorder()
	server.lspBroker = NoopLSPBroker{}
	mux.ServeHTTP(hoverRec, hoverReq)

	if hoverRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", hoverRec.Code)
	}

	snapshot := server.metrics.snapshot(0)
	requestCounts, ok := snapshot["lsp_request_counts"].(map[string]int64)
	if !ok {
		t.Fatalf("expected lsp_request_counts in snapshot, got %#v", snapshot["lsp_request_counts"])
	}
	if requestCounts["diagnostics"] != 1 || requestCounts["hover"] != 1 {
		t.Fatalf("unexpected lsp request counts: %+v", requestCounts)
	}

	errorCounts, ok := snapshot["lsp_error_counts"].(map[string]int64)
	if !ok {
		t.Fatalf("expected lsp_error_counts in snapshot, got %#v", snapshot["lsp_error_counts"])
	}
	if errorCounts[string(LSPErrorServerUnavailable)] != 1 {
		t.Fatalf("expected one typed lsp error, got %+v", errorCounts)
	}
}
