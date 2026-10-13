package sessionevents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Uses the production Python publisher against an actual broker, rather than
// duplicating its serialization/routing contract in a Go fake publisher.
func pythonPublish(t *testing.T, brokerURL, exchange, sessionID string, sequence int) {
	t.Helper()
	root, err := filepath.Abs("../../../../media_agents")
	if err != nil {
		t.Fatal(err)
	}
	python := os.Getenv("MEDIA_AGENTS_TEST_PYTHON")
	if python == "" {
		python = filepath.Join(root, ".venv", "bin", "python")
	}
	body, _ := json.Marshal(map[string]any{"session_id": sessionID, "sequence": sequence, "payload": map[string]any{"type": "test.event", "text": "中文"}})
	command := exec.Command(python, "-c", `
import asyncio, json, os
from adapter.session_event_bus import RabbitMQSessionEventBus
from adapter.biz.domain.runtime_session.event_bus import SessionEventNotification
async def main():
    bus = RabbitMQSessionEventBus(os.environ['TEST_BROKER_URL'], os.environ['TEST_EXCHANGE'])
    try:
        await bus.publish(SessionEventNotification(**json.loads(os.environ['TEST_BODY'])))
    finally:
        await bus.close()
asyncio.run(main())
`)
	command.Dir = root
	command.Env = append(os.Environ(), "TEST_BROKER_URL="+brokerURL, "TEST_EXCHANGE="+exchange, "TEST_BODY="+string(body))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("Python publisher failed: %v %s", err, output)
	}
}

func expectNotification(t *testing.T, subscription Subscription, sequence int64) {
	t.Helper()
	select {
	case event, ok := <-subscription.Events:
		if !ok || event.Sequence != sequence {
			t.Fatalf("notification=%+v open=%t, want sequence %d", event, ok, sequence)
		}
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil || payload["text"] != "中文" {
			t.Fatalf("invalid cross-language payload: %s", event.Payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("notification did not arrive")
	}
}

func assertQueueCount(t *testing.T, expected int) {
	t.Helper()
	base := os.Getenv("MEDIA_AGENTS_TEST_RABBITMQ_MANAGEMENT_URL")
	if base == "" {
		return
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("invalid management test URL")
	}
	u.Path = "/api/queues"
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(u.String())
	if err != nil {
		t.Fatal("RabbitMQ management unavailable")
	}
	defer response.Body.Close()
	var queues []struct{ Name string }
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&queues) != nil {
		t.Fatal("cannot read broker queue topology")
	}
	if len(queues) != expected {
		t.Fatalf("broker queue count %d, want %d", len(queues), expected)
	}
}

func TestRabbitSourceCrossLanguageFanOutAndReconnect(t *testing.T) {
	brokerURL := os.Getenv("MEDIA_AGENTS_TEST_RABBITMQ_URL")
	if brokerURL == "" {
		t.Skip("MEDIA_AGENTS_TEST_RABBITMQ_URL is required for RabbitMQ integration")
	}
	exchange := "test.gateway." + t.Name()
	one, err := NewRabbitSource(brokerURL, exchange, 16)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = one.Close() })
	two, err := NewRabbitSource(brokerURL, exchange, 16)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = two.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Topic punctuation must remain literal, including wildcard characters.
	sessionID := "session.*.#.会话"
	first, err := one.Open(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	local, err := one.Open(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	other, err := two.Open(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	unrelated, err := one.Open(ctx, "unrelated")
	if err != nil {
		t.Fatal(err)
	}
	defer unrelated.Close()
	assertQueueCount(t, 2) // Four local viewers still create only two queues.
	pythonPublish(t, brokerURL, exchange, sessionID, 9)
	for _, sub := range []Subscription{first, local, other} {
		expectNotification(t, sub, 9)
	}
	select {
	case event := <-unrelated.Events:
		t.Fatalf("unrelated Session received %v", event)
	default:
	}
	// Closing just one local viewer must leave the remaining binding intact.
	local.Close()
	pythonPublish(t, brokerURL, exchange, sessionID, 0)
	expectNotification(t, first, 0)
	expectNotification(t, other, 0)

	one.mu.Lock()
	old := one.current
	one.mu.Unlock()
	_ = old.Close()
	deadline := time.Now().Add(6 * time.Second)
	for {
		one.mu.Lock()
		ready := one.current != nil && one.current != old && one.bound[sessionID] && one.bound["unrelated"]
		one.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Gateway did not restore active bindings")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pythonPublish(t, brokerURL, exchange, sessionID, 10)
	expectNotification(t, first, 10) // Same local subscription survives reconnect.
	expectNotification(t, other, 10)
	assertQueueCount(t, 2)
	unrelated.Close()
	first.Close()
	deadline = time.Now().Add(3 * time.Second)
	for {
		one.mu.Lock()
		released := len(one.refs) == 0 && len(one.bound) == 0
		one.mu.Unlock()
		if released {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("last receiver did not release broker bindings")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
