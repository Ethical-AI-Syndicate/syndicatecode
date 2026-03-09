package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *APIClient) ListSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	if err := c.doJSON(ctx, http.MethodGet, "/sessions", nil, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (c *APIClient) CreateSession(ctx context.Context, req CreateSessionRequest) (*Session, error) {
	var session Session
	if err := c.doJSON(ctx, http.MethodPost, "/sessions", req, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *APIClient) CreateTurn(ctx context.Context, sessionID string, req CreateTurnRequest) (*Turn, error) {
	var turn Turn
	path := fmt.Sprintf("/sessions/%s/turns", sessionID)
	if err := c.doJSON(ctx, http.MethodPost, path, req, &turn); err != nil {
		return nil, err
	}
	return &turn, nil
}

func (c *APIClient) ListApprovals(ctx context.Context) ([]Approval, error) {
	var approvals []Approval
	if err := c.doJSON(ctx, http.MethodGet, "/approvals", nil, &approvals); err != nil {
		return nil, err
	}
	return approvals, nil
}

func (c *APIClient) DecideApproval(ctx context.Context, approvalID string, req DecideApprovalRequest) (*Approval, error) {
	var approval Approval
	path := fmt.Sprintf("/approvals/%s", approvalID)
	if err := c.doJSON(ctx, http.MethodPost, path, req, &approval); err != nil {
		return nil, err
	}
	return &approval, nil
}

func (c *APIClient) GetTurnContext(ctx context.Context, sessionID, turnID string) ([]ContextFragment, error) {
	var fragments []ContextFragment
	path := fmt.Sprintf("/sessions/%s/turns/%s/context", sessionID, turnID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &fragments); err != nil {
		return nil, err
	}
	return fragments, nil
}

func (c *APIClient) doJSON(ctx context.Context, method, path string, in interface{}, out interface{}) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(payload)
	}

	url := c.baseURL + "/api/v1" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error (%d): %s", resp.StatusCode, strings.TrimSpace(string(message)))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
