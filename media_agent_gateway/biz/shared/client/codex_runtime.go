package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type HTTPRuntime struct {
	AgentType string
	BaseURL   string
	HTTP      *http.Client
}

type APIError struct {
	AgentType  string
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s Agent API returned %s: %s", e.AgentType, e.Status, e.Body)
}

func IsDefinitiveRegistrationFailure(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != http.StatusRequestTimeout && apiErr.StatusCode != http.StatusTooManyRequests
}

func (r *HTTPRuntime) RegisterRuntimeSession(ctx context.Context, input RegisterRuntimeSessionInput) (*RuntimeSession, error) {
	var output RuntimeSession
	if err := r.doJSON(ctx, http.MethodPost, "/api/v1/runtime-sessions", input, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *HTTPRuntime) GetRuntimeSession(ctx context.Context, sessionID string) (*RuntimeSession, error) {
	var output RuntimeSession
	if err := r.doJSON(ctx, http.MethodGet, "/api/v1/runtime-sessions/"+url.PathEscape(sessionID), nil, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *HTTPRuntime) SubmitTurn(ctx context.Context, input SubmitTurnInput) (*Turn, error) {
	var output Turn
	path := "/api/v1/runtime-sessions/" + url.PathEscape(input.SessionID) + "/turns"
	if err := r.doJSON(ctx, http.MethodPost, path, input, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *HTTPRuntime) InterruptTurn(ctx context.Context, sessionID, turnID string) (bool, error) {
	var output struct {
		Interrupted bool `json:"interrupted"`
	}
	path := "/api/v1/runtime-sessions/" + url.PathEscape(sessionID) + "/turns/" + url.PathEscape(turnID) + "/interrupt"
	if err := r.doJSON(ctx, http.MethodPost, path, map[string]any{}, &output); err != nil {
		return false, err
	}
	return output.Interrupted, nil
}

func (r *HTTPRuntime) ListMessages(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	var output []Message
	path := fmt.Sprintf("/api/v1/runtime-sessions/%s/messages?limit=%d", url.PathEscape(sessionID), limit)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (r *HTTPRuntime) ReplayEvents(ctx context.Context, sessionID string, afterSequence int64, limit int) ([]UniversalEvent, error) {
	var output []UniversalEvent
	path := fmt.Sprintf("/api/v1/runtime-sessions/%s/events?after_sequence=%d&limit=%d", url.PathEscape(sessionID), afterSequence, limit)
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (r *HTTPRuntime) ListAgentRequests(ctx context.Context, sessionID, turnID string) ([]AgentRequest, error) {
	var output []AgentRequest
	query := url.Values{}
	if turnID != "" {
		query.Set("turn_id", turnID)
	}
	path := "/api/v1/runtime-sessions/" + url.PathEscape(sessionID) + "/requests"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	if err := r.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return output, nil
}

func (r *HTTPRuntime) ResolveAgentRequest(ctx context.Context, sessionID, turnID, requestID string, response map[string]any) (*AgentRequest, error) {
	var output AgentRequest
	path := "/api/v1/runtime-sessions/" + url.PathEscape(sessionID) + "/turns/" + url.PathEscape(turnID) + "/requests/" + url.PathEscape(requestID) + "/resolve"
	if err := r.doJSON(ctx, http.MethodPost, path, map[string]any{"response": response}, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (r *HTTPRuntime) doJSON(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	httpClient := r.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return &APIError{AgentType: r.AgentType, StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(data))}
	}
	return json.NewDecoder(resp.Body).Decode(output)
}
