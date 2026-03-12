package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
)

func (s *Server) ensureLSPBroker() LSPBroker {
	if s.lspBroker != nil {
		return s.lspBroker
	}
	return NoopLSPBroker{}
}

func (s *Server) handleLSPDiagnostics(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var callErr error
	defer func() { s.recordLSPRouteCall("diagnostics", started, callErr) }()

	if r.Method != http.MethodGet {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if sessionID == "" || path == "" {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusBadRequest, false, "session_id and path are required", nil)
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, sessionID)
	if err != nil {
		callErr = err
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	diagnostics, err := s.ensureLSPBroker().Diagnostics(r.Context(), repoPath, path)
	if err != nil {
		callErr = err
		if writeLSPError(w, err) {
			return
		}
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.appendLSPAuditEvent(r.Context(), sessionID, "diagnostics", path)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"diagnostics": diagnostics})
}

func (s *Server) handleLSPSymbols(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	var callErr error
	defer func() { s.recordLSPRouteCall("symbols", started, callErr) }()

	if r.Method != http.MethodGet {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if sessionID == "" || path == "" {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusBadRequest, false, "session_id and path are required", nil)
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, sessionID)
	if err != nil {
		callErr = err
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	symbols, err := s.ensureLSPBroker().Symbols(r.Context(), repoPath, path)
	if err != nil {
		callErr = err
		if writeLSPError(w, err) {
			return
		}
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.appendLSPAuditEvent(r.Context(), sessionID, "symbols", path)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"symbols": symbols})
}

func (s *Server) handleLSPHover(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req lspPositionRequest) (interface{}, error) {
		return s.ensureLSPBroker().Hover(r.Context(), repoPath, req)
	})
}

func (s *Server) handleLSPDefinition(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req lspPositionRequest) (interface{}, error) {
		locations, err := s.ensureLSPBroker().Definition(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"locations": locations}, nil
	})
}

func (s *Server) handleLSPReferences(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req lspPositionRequest) (interface{}, error) {
		locations, err := s.ensureLSPBroker().References(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"locations": locations}, nil
	})
}

func (s *Server) handleLSPCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req lspPositionRequest) (interface{}, error) {
		items, err := s.ensureLSPBroker().Completions(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"items": items}, nil
	})
}

func (s *Server) handleLSPPositionRequest(w http.ResponseWriter, r *http.Request, resolve func(repoPath string, req lspPositionRequest) (interface{}, error)) {
	method := strings.TrimPrefix(r.URL.Path, "/api/v1/lsp/")
	started := time.Now()
	var callErr error
	defer func() { s.recordLSPRouteCall(method, started, callErr) }()

	if r.Method != http.MethodPost {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusMethodNotAllowed, false, "method not allowed", nil)
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req lspPositionRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Path) == "" {
		callErr = newLSPError(LSPErrorBackendUnhealthy, http.StatusBadRequest, false, "session_id and path are required", nil)
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, req.SessionID)
	if err != nil {
		callErr = err
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out, err := resolve(repoPath, req)
	if err != nil {
		callErr = err
		if writeLSPError(w, err) {
			return
		}
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.appendLSPAuditEvent(r.Context(), req.SessionID, method, req.Path)
	_ = json.NewEncoder(w).Encode(out)
}

func writeLSPError(w http.ResponseWriter, err error) bool {
	lspErr, ok := asLSPError(err)
	if !ok {
		return false
	}
	status := lspErr.Status
	if status == 0 {
		status = http.StatusBadGateway
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Type:      string(lspErr.Code),
		Reason:    lspErr.Reason,
		Retryable: lspErr.Retryable,
		Details:   lspErr.Details,
	})
	return true
}

func (s *Server) recordLSPRouteCall(method string, started time.Time, callErr error) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.recordLSPRequest(method, time.Since(started))
	if lspErr, ok := asLSPError(callErr); ok {
		s.metrics.recordLSPError(string(lspErr.Code))
	}
}

func (s *Server) appendLSPAuditEvent(ctx context.Context, sessionID, method, path string) {
	if s == nil || s.eventStore == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	payload, err := json.Marshal(map[string]string{"method": method, "path": path})
	if err != nil {
		return
	}
	_ = s.eventStore.Append(ctx, audit.Event{
		ID:        uuid.NewString(),
		SessionID: sessionID,
		Timestamp: time.Now().UTC(),
		EventType: audit.EventLSPRequest,
		Actor:     requestActor(ctx),
		Payload:   payload,
	})
}
