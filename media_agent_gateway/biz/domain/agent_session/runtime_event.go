package agent_session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const DefaultMaxToolResultBytes = 32 * 1024

type RuntimeEvent struct {
	Type      string
	SessionID string
	ItemID    string
	ItemKind  string
	CallID    string
}

func ParseRuntimeEvent(raw json.RawMessage) (RuntimeEvent, error) {
	var envelope struct {
		Type      string `json:"type"`
		SessionID string `json:"session_id"`
		ItemID    string `json:"item_id"`
		Item      *struct {
			ItemID  string           `json:"item_id"`
			Kind    any              `json:"kind"`
			Content []map[string]any `json:"content"`
		} `json:"item"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return RuntimeEvent{}, err
	}
	event := RuntimeEvent{Type: envelope.Type, SessionID: envelope.SessionID, ItemID: envelope.ItemID}
	if envelope.Item == nil {
		return event, nil
	}
	if envelope.Item.ItemID != "" {
		event.ItemID = envelope.Item.ItemID
	}
	event.ItemKind = protocolName(envelope.Item.Kind)
	for _, part := range envelope.Item.Content {
		if callID, ok := part["call_id"].(string); ok && callID != "" {
			event.CallID = callID
			break
		}
	}
	return event, nil
}

func (e RuntimeEvent) IsTransient() bool {
	return e.Type == "item.started" || e.Type == "item.delta"
}

func (e RuntimeEvent) IsToolResult() bool {
	return e.Type == "item.completed" && e.ItemKind == "tool_result"
}

func (e RuntimeEvent) IsToolCall() bool {
	return e.Type == "item.completed" && e.ItemKind == "tool_call"
}

func (e RuntimeEvent) CreatesMessage() bool {
	return e.Type == "item.completed" && e.ItemKind == "message"
}

func (e RuntimeEvent) IsTerminal() bool { return e.Type == "session.ended" }

func NormalizeToolResult(raw json.RawMessage, maxBytes int) (json.RawMessage, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxToolResultBytes
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	item, ok := envelope["item"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool result event has no item")
	}
	content, ok := item["content"].([]any)
	if !ok {
		return nil, fmt.Errorf("tool result item has no content")
	}
	for _, value := range content {
		part, ok := value.(map[string]any)
		if !ok || protocolName(part["type"]) != "tool_result" {
			continue
		}
		output, _ := part["output"].(string)
		if truncated, _ := part["truncated"].(bool); truncated && len([]byte(output)) <= maxBytes {
			continue
		}
		originalBytes := len([]byte(output))
		normalized, truncated, _ := truncateUTF8(strings.ReplaceAll(output, "\x00", "\uFFFD"), maxBytes)
		part["output"] = normalized
		part["truncated"] = truncated
		part["original_bytes"] = originalBytes
		part["stored_bytes"] = len([]byte(normalized))
	}
	return json.Marshal(envelope)
}

func protocolName(value any) string {
	name := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	return strings.ReplaceAll(name, "-", "_")
}

func truncateUTF8(value string, maxBytes int) (string, bool, int) {
	original := len([]byte(value))
	if original <= maxBytes {
		return value, false, original
	}
	marker := fmt.Sprintf("\n...[truncated: %d bytes omitted, original %d bytes]...\n", original-maxBytes, original)
	markerBytes := len([]byte(marker))
	if markerBytes >= maxBytes {
		return validPrefix(value, maxBytes), true, original
	}
	remaining := maxBytes - markerBytes
	headBudget := remaining * 3 / 4
	tailBudget := remaining - headBudget
	head := validPrefix(value, headBudget)
	tail := validSuffix(value, tailBudget)
	result := head + marker + tail
	if len([]byte(result)) > maxBytes {
		result = validPrefix(result, maxBytes)
	}
	return result, true, original
}

func validPrefix(value string, maxBytes int) string {
	data := []byte(value)
	if len(data) <= maxBytes {
		return value
	}
	data = data[:maxBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}

func validSuffix(value string, maxBytes int) string {
	data := []byte(value)
	if len(data) <= maxBytes {
		return value
	}
	data = data[len(data)-maxBytes:]
	for len(data) > 0 && !utf8.Valid(data) {
		_, size := utf8.DecodeRune(data)
		if size <= 0 || size > len(data) {
			data = data[1:]
		} else {
			data = data[size:]
		}
	}
	return string(bytes.Clone(data))
}
