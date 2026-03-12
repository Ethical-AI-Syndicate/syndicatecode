package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

type schemaErrorResponse struct {
	Error      string            `json:"error"`
	Violations []schemaViolation `json:"violations"`
}

func TestSchemaValidationMiddleware_RejectsInvalidRequest(t *testing.T) {
	nextCalled := false
	handler := schemaValidationMiddleware(
		map[string]jsonObjectSchema{
			http.MethodPost: {
				"repo_path":  {Type: schemaTypeString, Required: true},
				"trust_tier": {Type: schemaTypeString, Required: true},
			},
		},
		nil,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusCreated)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(`{"repo_path":123}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("expected next handler not to be called for invalid request")
	}

	var resp schemaErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode schema error response: %v", err)
	}
	if resp.Error != "invalid_schema" {
		t.Fatalf("expected invalid_schema error, got %s", resp.Error)
	}
	if len(resp.Violations) == 0 {
		t.Fatal("expected at least one schema violation")
	}
}

func TestSchemaValidationMiddleware_RejectsInvalidResponse(t *testing.T) {
	handler := schemaValidationMiddleware(
		nil,
		map[string]jsonObjectSchema{
			http.MethodPost: {
				"session_id": {Type: schemaTypeString, Required: true},
			},
		},
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{"session_id": 123}); err != nil {
				t.Fatalf("failed to encode response: %v", err)
			}
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(`{"repo_path":"/tmp/repo","trust_tier":"tier1"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for invalid response schema, got %d", rec.Code)
	}

	var resp schemaErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode schema error response: %v", err)
	}
	if resp.Error != "invalid_schema" {
		t.Fatalf("expected invalid_schema error, got %s", resp.Error)
	}
}

func TestSessionsEndpoint_ReturnsInvalidSchemaForMissingFields(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	server := &Server{
		sessionMgr: session.NewManager(eventStore),
		eventStore: eventStore,
		httpServer: &http.Server{},
	}

	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(`{"repo_path":"/tmp/repo"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp schemaErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode schema error response: %v", err)
	}
	if resp.Error != "invalid_schema" {
		t.Fatalf("expected invalid_schema error, got %s", resp.Error)
	}
}

func TestLSPHoverEndpoint_ReturnsInvalidSchemaForMissingPosition(t *testing.T) {
	eventStore, err := audit.NewEventStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("failed to create event store: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := eventStore.Close(); closeErr != nil {
			t.Fatalf("failed to close event store: %v", closeErr)
		}
	})

	sessionMgr := session.NewManager(eventStore)
	created, err := sessionMgr.Create(context.Background(), t.TempDir(), "tier1")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	server := &Server{
		sessionMgr: sessionMgr,
		eventStore: eventStore,
		httpServer: &http.Server{},
		lspBroker:  NoopLSPBroker{},
	}

	mux := http.NewServeMux()
	server.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/lsp/hover", bytes.NewBufferString(`{"session_id":"`+created.ID+`","path":"main.go","col":1}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp schemaErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode schema error response: %v", err)
	}
	if resp.Error != "invalid_schema" {
		t.Fatalf("expected invalid_schema error, got %s", resp.Error)
	}
}
