package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleEventTypes_Bead_l3d_17_1(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/event_types", nil)
	w := httptest.NewRecorder()

	s := &Server{}
	s.handleEventTypes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}
