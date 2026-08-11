package client

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	sharedsse "media_agent/media_agent_gateway/biz/shared/sse"
)

type ChatInput struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
	APIKey    string `json:"api_key,omitempty"`
}

type Registry map[string]Runtime

func (r Registry) Get(agentType string) (Runtime, bool) { value, ok := r[agentType]; return value, ok }

func NewRuntimeRegistryFromEnvironment() Registry {
	return Registry{"codex": &CodexRuntime{BaseURL: envValue("CODEX_RUNTIME_URL", "http://127.0.0.1:3001"), SSE: &sharedsse.Client{HTTP: &http.Client{}}}}
}

func envValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// Runtime is the Agent-independent streaming boundary used by workflows.
// Codex, Goose, Claude, and future clients implement only request differences.
type Runtime interface {
	Chat(context.Context, ChatInput, func(json.RawMessage) error) error
}
