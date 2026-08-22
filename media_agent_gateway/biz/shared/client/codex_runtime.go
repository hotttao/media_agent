package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sharedsse "media_agent/media_agent_gateway/biz/shared/sse"
)

type HTTPRuntime struct {
	AgentType string
	BaseURL   string
	SSE       *sharedsse.Client
	HTTP      *http.Client
}

func (r *HTTPRuntime) Interrupt(ctx context.Context, runtimeSessionID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.BaseURL, "/")+"/sessions/"+runtimeSessionID+"/interrupt", nil)
	if err != nil {
		return false, err
	}
	httpClient := r.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%s runtime interrupt returned %s", r.AgentType, resp.Status)
	}
	var result struct {
		Success     bool `json:"success"`
		Interrupted bool `json:"interrupted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Success && result.Interrupted, nil
}

func (*HTTPRuntime) EventPersistenceCapabilities() EventPersistenceCapabilities {
	return EventPersistenceCapabilities{MaxToolResultBytes: 32 * 1024}
}

func (r *HTTPRuntime) Chat(ctx context.Context, input ChatInput, consume func(json.RawMessage) error) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(r.BaseURL, "/")+"/chat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	transport := r.SSE
	if transport == nil {
		transport = &sharedsse.Client{}
	}
	return transport.Stream(ctx, req, func(event sharedsse.Event) error {
		raw := json.RawMessage(event.Data)
		if !json.Valid(raw) {
			return fmt.Errorf("%s runtime emitted invalid Universal Event JSON", r.AgentType)
		}
		return consume(raw)
	})
}
