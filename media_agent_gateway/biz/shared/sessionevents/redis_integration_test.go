package sessionevents

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisSourceBroadcastsToIndependentGatewayInstances(t *testing.T) {
	if _, err := exec.LookPath("redis-server"); err != nil {
		t.Skip("redis-server is not installed")
	}
	socket := filepath.Join(t.TempDir(), "redis.sock")
	command := exec.Command("redis-server", "--port", "0", "--save", "", "--appendonly", "no", "--unixsocket", socket, "--unixsocketperm", "700")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	url := "unix://" + socket
	var publisher *redis.Client
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		options, _ := redis.ParseURL(url)
		publisher = redis.NewClient(options)
		if publisher.Ping(context.Background()).Err() == nil {
			break
		}
		_ = publisher.Close()
		time.Sleep(10 * time.Millisecond)
	}
	if publisher == nil || publisher.Ping(context.Background()).Err() != nil {
		t.Fatal("Redis did not become ready")
	}
	t.Cleanup(func() { _ = publisher.Close() })
	one, err := NewRedisSource(url, "test-events", 8)
	if err != nil {
		t.Fatal(err)
	}
	two, err := NewRedisSource(url, "test-events", 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = one.Close(); _ = two.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	subOne, err := one.Open(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	subTwo, err := two.Open(ctx, "session")
	if err != nil {
		t.Fatal(err)
	}
	defer subOne.Close()
	defer subTwo.Close()
	payload, _ := json.Marshal(map[string]any{"session_id": "session", "sequence": 9, "payload": json.RawMessage(`{"type":"turn.completed"}`)})
	if err := publisher.Publish(ctx, "test-events:session", payload).Err(); err != nil {
		t.Fatal(err)
	}
	for name, subscription := range map[string]Subscription{"one": subOne, "two": subTwo} {
		select {
		case event := <-subscription.Events:
			if event.Sequence != 9 || string(event.Payload) != `{"type":"turn.completed"}` {
				t.Fatalf("%s event=%+v", name, event)
			}
		case <-ctx.Done():
			t.Fatalf("%s missed broadcast: %v", name, ctx.Err())
		}
	}
}
