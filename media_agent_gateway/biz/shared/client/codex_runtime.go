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

type CodexRuntime struct {
	BaseURL string
	SSE     *sharedsse.Client
}

func (*CodexRuntime) EventPersistenceCapabilities() EventPersistenceCapabilities {
	return EventPersistenceCapabilities{MaxToolResultBytes: 32 * 1024}
}

func (r *CodexRuntime) Chat(ctx context.Context, input ChatInput, consume func(json.RawMessage) error) error {
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
			return fmt.Errorf("codex runtime emitted invalid Universal Event JSON")
		}
		return consume(raw)
	})
}
