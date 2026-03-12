package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/session"
	api "gitlab.mikeholownych.com/ai-syndicate/syndicatecode/pkg/api"
)

func (s *Server) ensureLSPBroker() LSPBroker {
	if s.lspBroker != nil {
		return s.lspBroker
	}
	return NoopLSPBroker{}
}

func (s *Server) handleLSPDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if sessionID == "" || path == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, sessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	diagnostics, err := s.ensureLSPBroker().Diagnostics(r.Context(), repoPath, path)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"diagnostics": diagnostics})
}

func (s *Server) handleLSPSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if sessionID == "" || path == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, sessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	symbols, err := s.ensureLSPBroker().Symbols(r.Context(), repoPath, path)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"symbols": symbols})
}

func (s *Server) handleLSPHover(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req api.LSPPositionRequest) (interface{}, error) {
		return s.ensureLSPBroker().Hover(r.Context(), repoPath, req)
	})
}

func (s *Server) handleLSPDefinition(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req api.LSPPositionRequest) (interface{}, error) {
		locations, err := s.ensureLSPBroker().Definition(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"locations": locations}, nil
	})
}

func (s *Server) handleLSPReferences(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req api.LSPPositionRequest) (interface{}, error) {
		locations, err := s.ensureLSPBroker().References(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"locations": locations}, nil
	})
}

func (s *Server) handleLSPCompletions(w http.ResponseWriter, r *http.Request) {
	s.handleLSPPositionRequest(w, r, func(repoPath string, req api.LSPPositionRequest) (interface{}, error) {
		items, err := s.ensureLSPBroker().Completions(r.Context(), repoPath, req)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"items": items}, nil
	})
}

func (s *Server) handleLSPPositionRequest(w http.ResponseWriter, r *http.Request, resolve func(repoPath string, req api.LSPPositionRequest) (interface{}, error)) {
	if r.Method != http.MethodPost {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req api.LSPPositionRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Path) == "" {
		writeStatusError(w, http.StatusBadRequest, "session_id and path are required")
		return
	}

	repoPath, err := repoPathForSession(r.Context(), s.sessionMgr, req.SessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			writeStatusError(w, http.StatusNotFound, err.Error())
			return
		}
		writeStatusError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out, err := resolve(repoPath, req)
	if err != nil {
		writeStatusError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}
