package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
)

type ErrorEnvelope struct {
	Type      string            `json:"type"`
	Reason    string            `json:"reason"`
	Retryable bool              `json:"retryable,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

var (
	ErrNotFound         = errors.New("not found")
	ErrValidationFailed = errors.New("validation failed")
	ErrInternal         = errors.New("internal error")
	ErrRateLimited      = errors.New("rate limited")
	ErrPolicyDenied     = errors.New("policy denied")
)

func WriteError(w http.ResponseWriter, status int, err error) {
	envelope := ErrorEnvelope{
		Reason: err.Error(),
	}

	switch status {
	case http.StatusNotFound:
		envelope.Type = "not_found"
		envelope.Retryable = false
	case http.StatusBadRequest:
		envelope.Type = "validation_failed"
		envelope.Retryable = false
	case http.StatusForbidden:
		envelope.Type = "policy_denied"
		envelope.Retryable = false
	case http.StatusTooManyRequests:
		envelope.Type = "rate_limited"
		envelope.Retryable = true
	case http.StatusInternalServerError:
		envelope.Type = "internal_error"
		envelope.Retryable = true
	default:
		envelope.Type = "error"
		envelope.Retryable = status >= 500
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		w.Write([]byte(`{"type":"error","reason":"failed to encode error"}`)) //nolint:errcheck
	}
}
