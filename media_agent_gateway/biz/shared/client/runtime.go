package client

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
)

type Registry map[string]Runtime

func (r Registry) Get(agentType string) (Runtime, bool) { value, ok := r[agentType]; return value, ok }

func NewRuntimeRegistryFromEnvironment() Registry {
	return Registry{
		"codex":       &HTTPRuntime{AgentType: "codex", BaseURL: envValue("CODEX_RUNTIME_URL", "http://127.0.0.1:3001"), HTTP: &http.Client{}},
		"claude-code": &HTTPRuntime{AgentType: "claude-code", BaseURL: envValue("CLAUDE_CODE_RUNTIME_URL", "http://127.0.0.1:3003"), HTTP: &http.Client{}},
	}
}

func envValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// Runtime is the Agent-owned Runtime Session API used by Gateway workflows.
type Runtime interface {
	RegisterRuntimeSession(context.Context, RegisterRuntimeSessionInput) (*RuntimeSession, error)
	GetRuntimeSession(context.Context, string) (*RuntimeSession, error)
	SubmitTurn(context.Context, SubmitTurnInput) (*Turn, error)
	GetTurn(context.Context, string, string) (*Turn, error)
	InterruptTurn(context.Context, string, string) (bool, error)
	ListMessages(context.Context, string, int) ([]Message, error)
	ReplayEvents(context.Context, string, int64, int) ([]UniversalEvent, error)
	ListAgentRequests(context.Context, string, string) ([]AgentRequest, error)
	ResolveAgentRequest(context.Context, string, string, string, map[string]any) (*AgentRequest, error)
}

type RegisterRuntimeSessionInput struct {
	SessionID      string `json:"session_id"`
	ContextVersion int64  `json:"context_version"`
	ContextRef     string `json:"context_ref"`
}

type RuntimeSession struct {
	SessionID        string `json:"session_id"`
	Status           string `json:"status"`
	ChatStatus       string `json:"chat_status"`
	ActiveTurnID     string `json:"active_turn_id"`
	RuntimeSessionID string `json:"runtime_session_id"`
	ContextVersion   int64  `json:"context_version"`
}

type SubmitTurnInput struct {
	SessionID      string         `json:"-"`
	IdempotencyKey string         `json:"idempotency_key"`
	ContextVersion int64          `json:"context_version"`
	Input          map[string]any `json:"input"`
	APIKey         string         `json:"api_key,omitempty"`
	SessionToken   string         `json:"session_token,omitempty"`
}

type Turn struct {
	TurnID          string `json:"turn_id"`
	SessionID       string `json:"session_id"`
	IdempotencyKey  string `json:"idempotency_key"`
	EnqueueSequence int64  `json:"enqueue_sequence"`
	Status          string `json:"status"`
	ErrorClass      string `json:"error_class"`
}

type Message struct {
	MessageID  string          `json:"message_id"`
	SessionID  string          `json:"session_id"`
	TurnID     string          `json:"turn_id"`
	Role       string          `json:"role"`
	ContentRef json.RawMessage `json:"content_ref"`
	Sequence   int64           `json:"sequence"`
	CreatedAt  string          `json:"created_at"`
}

type UniversalEvent struct {
	EventID   string          `json:"event_id"`
	SessionID string          `json:"session_id"`
	TurnID    string          `json:"turn_id"`
	Sequence  int64           `json:"sequence"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
}

type AgentRequest struct {
	RequestID   string          `json:"request_id"`
	SessionID   string          `json:"session_id"`
	TurnID      string          `json:"turn_id"`
	RequestType string          `json:"request_type"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	Response    json.RawMessage `json:"response"`
	ResolvedAt  string          `json:"resolved_at"`
	CreatedAt   string          `json:"created_at"`
}
