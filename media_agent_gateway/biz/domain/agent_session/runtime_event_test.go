package agent_session

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRuntimeEventClassification(t *testing.T) {
	raw := json.RawMessage(`{"type":"item.completed","item":{"item_id":"result-1","kind":"tool_result","content":[{"type":"tool_result","call_id":"call-1","output":"ok"}]}}`)
	event, err := ParseRuntimeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !event.IsToolResult() || event.CallID != "call-1" || event.ItemID != "result-1" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if (RuntimeEvent{Type: "item.delta"}).IsTransient() != true {
		t.Fatal("item.delta must be transient")
	}
}

func TestNormalizeToolResultTruncatesOnceWithValidUTF8(t *testing.T) {
	output := strings.Repeat("界", 100) + "\x00tail"
	raw, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"kind": "tool_result", "content": []any{map[string]any{"type": "tool_result", "call_id": "call", "output": output}}}})
	normalized, err := NormalizeToolResult(raw, 96)
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(normalized, &event); err != nil {
		t.Fatal(err)
	}
	part := event["item"].(map[string]any)["content"].([]any)[0].(map[string]any)
	stored := part["output"].(string)
	if !utf8.ValidString(stored) || strings.ContainsRune(stored, '\x00') {
		t.Fatalf("output was not safely normalized: %q", stored)
	}
	if part["truncated"] != true || int(part["original_bytes"].(float64)) != len([]byte(output)) {
		t.Fatalf("missing truncation metadata: %+v", part)
	}
	if len([]byte(stored)) > 96 || !strings.HasSuffix(stored, "tail") {
		t.Fatalf("unexpected retained output: bytes=%d value=%q", len([]byte(stored)), stored)
	}
	twice, err := NormalizeToolResult(normalized, 96)
	if err != nil {
		t.Fatal(err)
	}
	if string(twice) != string(normalized) {
		t.Fatalf("tool result was truncated more than once:\nfirst:  %s\nsecond: %s", normalized, twice)
	}
}
