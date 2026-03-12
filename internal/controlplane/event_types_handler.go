package controlplane

import (
	"encoding/json"
	"log"
	"net/http"

	"gitlab.mikeholownych.com/ai-syndicate/syndicatecode/internal/audit"
)

func (s *Server) handleEventTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeStatusError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"event_types": audit.KnownEventTypes()}); err != nil {
		log.Printf("failed to encode event types response: %v", err)
	}
}
