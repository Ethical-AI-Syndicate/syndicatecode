package controlplane

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type TestErrorEnvelope struct {
	Type      string            `json:"type"`
	Reason    string            `json:"reason"`
	Retryable bool              `json:"retryable,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

func TestErrorEnvelope_Bead_l3d_2_3(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		statusCode    int
		expectedType  string
		expectedRetry bool
	}{
		{
			name:          "not found error is not retryable",
			err:           ErrNotFound,
			statusCode:    http.StatusNotFound,
			expectedType:  "not_found",
			expectedRetry: false,
		},
		{
			name:          "validation error is not retryable",
			err:           ErrValidationFailed,
			statusCode:    http.StatusBadRequest,
			expectedType:  "validation_failed",
			expectedRetry: false,
		},
		{
			name:          "internal error is retryable",
			err:           ErrInternal,
			statusCode:    http.StatusInternalServerError,
			expectedType:  "internal_error",
			expectedRetry: true,
		},
		{
			name:          "rate limited error is retryable",
			err:           ErrRateLimited,
			statusCode:    http.StatusTooManyRequests,
			expectedType:  "rate_limited",
			expectedRetry: true,
		},
		{
			name:          "policy denied error is not retryable",
			err:           ErrPolicyDenied,
			statusCode:    http.StatusForbidden,
			expectedType:  "policy_denied",
			expectedRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockResponseWriter{}
			WriteError(w, tt.statusCode, tt.err)

			if w.statusCode != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, w.statusCode)
			}

			var envelope ErrorEnvelope
			if err := json.Unmarshal(w.body, &envelope); err != nil {
				t.Fatalf("failed to unmarshal error envelope: %v", err)
			}

			if envelope.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, envelope.Type)
			}

			if envelope.Retryable != tt.expectedRetry {
				t.Errorf("expected retryable %v, got %v", tt.expectedRetry, envelope.Retryable)
			}
		})
	}
}

func TestWriteError_DoesNotExposeInternalDetails_Bead_l3d_2_3(t *testing.T) {
	w := &mockResponseWriter{}
	internalErr := &customError{internalMsg: "secret key 12345", publicMsg: "operation failed"}
	WriteError(w, http.StatusInternalServerError, internalErr)

	var envelope TestErrorEnvelope
	if err := json.Unmarshal(w.body, &envelope); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	bodyStr := string(w.body)
	if strings.Contains(bodyStr, "secret") || strings.Contains(bodyStr, "12345") {
		t.Error("error response should not expose internal details")
	}
}

type customError struct {
	internalMsg string
	publicMsg   string
}

func (e *customError) Error() string {
	return e.publicMsg
}

type mockResponseWriter struct {
	statusCode int
	body       []byte
	header     http.Header
}

func (m *mockResponseWriter) Header() http.Header {
	if m.header == nil {
		m.header = make(http.Header)
	}
	return m.header
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(code int) {
	m.statusCode = code
}
