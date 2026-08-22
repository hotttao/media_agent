package sessionevents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

const defaultChannelPrefix = "media-agents:session-events"

type RedisSource struct {
	client  *redis.Client
	prefix  string
	buffer  int
	closeMu sync.Mutex
	closed  bool
}

func NewRedisSource(rawURL, prefix string, buffer int) (*RedisSource, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse Session Event Bus Redis URL: %w", err)
	}
	if prefix == "" {
		prefix = defaultChannelPrefix
	}
	if buffer <= 0 {
		buffer = defaultBuffer
	}
	return &RedisSource{client: redis.NewClient(options), prefix: prefix, buffer: buffer}, nil
}

func (s *RedisSource) Open(ctx context.Context, sessionID string) (Subscription, error) {
	pubsub := s.client.Subscribe(ctx, s.prefix+":"+sessionID)
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return Subscription{}, fmt.Errorf("subscribe Session Event Bus: %w", err)
	}
	subCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, s.buffer)
	var once sync.Once
	closeSubscription := func() {
		once.Do(func() {
			cancel()
			_ = pubsub.Close()
		})
	}
	go func() {
		defer close(events)
		defer closeSubscription()
		for {
			message, err := pubsub.ReceiveMessage(subCtx)
			if err != nil {
				return
			}
			var notification struct {
				SessionID string          `json:"session_id"`
				Sequence  int64           `json:"sequence"`
				Payload   json.RawMessage `json:"payload"`
			}
			if json.Unmarshal([]byte(message.Payload), &notification) != nil || notification.SessionID != sessionID || !json.Valid(notification.Payload) {
				continue
			}
			select {
			case events <- Event{Sequence: notification.Sequence, Payload: append([]byte(nil), notification.Payload...)}:
			case <-subCtx.Done():
				return
			default:
				return
			}
		}
	}()
	return Subscription{Events: events, cancel: closeSubscription}, nil
}

func (s *RedisSource) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.client.Close()
}
