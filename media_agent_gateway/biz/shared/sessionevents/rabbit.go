package sessionevents

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"media_agent/hertz_infra/messagebus"
)

const DefaultExchange = "media.agent.sessions.v1"

// RabbitSource multiplexes local subscribers over ONE ephemeral instance queue.
// refs are desired bindings; bound records acknowledged bindings on the current
// connection. Open waits for that acknowledgement before replay can begin.
type RabbitSource struct {
	mu      sync.Mutex
	refs    map[string]int
	bound   map[string]bool
	changed chan struct{}
	wake    chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc
	closed  bool
	current *messagebus.TopicConsumer
	local   *Broadcaster
}

func NewRabbitSource(rawURL, exchange string, buffer int) (*RabbitSource, error) {
	if messagebus.ValidateURL(rawURL) != nil {
		return nil, errors.New("invalid Session Event Bus RabbitMQ URL")
	}
	if exchange == "" {
		exchange = DefaultExchange
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &RabbitSource{
		refs: make(map[string]int), bound: make(map[string]bool),
		changed: make(chan struct{}), wake: make(chan struct{}, 1), done: make(chan struct{}),
		cancel: cancel, local: New(buffer),
	}
	go s.run(ctx, rawURL, exchange)
	return s, nil
}

func routingKey(sessionID string) string { return hex.EncodeToString([]byte(sessionID)) }

func (s *RabbitSource) Open(ctx context.Context, sessionID string) (Subscription, error) {
	// Topic routing keys are limited to 255 bytes. Reject ambiguous/oversized IDs.
	if sessionID == "" || len(sessionID) > 127 {
		return Subscription{}, errors.New("invalid Session Event Bus session ID")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Subscription{}, errors.New("Session Event Bus closed")
	}
	local := s.local.Subscribe(sessionID)
	s.refs[sessionID]++
	s.mu.Unlock()
	var once sync.Once
	closeSub := func() {
		once.Do(func() {
			local.Close()
			s.mu.Lock()
			s.refs[sessionID]--
			if s.refs[sessionID] == 0 {
				delete(s.refs, sessionID)
			}
			s.mu.Unlock()
			s.signal()
		})
	}
	s.signal()
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for {
		s.mu.Lock()
		ready, closed, changed := s.bound[sessionID], s.closed, s.changed
		s.mu.Unlock()
		if closed {
			closeSub()
			return Subscription{}, errors.New("Session Event Bus closed")
		}
		if ready {
			break
		}
		select {
		case <-waitCtx.Done():
			closeSub()
			return Subscription{}, fmt.Errorf("wait for Session Event Bus binding: %w", waitCtx.Err())
		case <-changed:
		}
	}
	// Local overflow closes its channel. EventStream closes/reopens that receiver;
	// cancellation must also release bindings for callers that only cancel ctx.
	stop := context.AfterFunc(ctx, closeSub)
	return Subscription{Events: local.Events, cancel: func() { stop(); closeSub() }}, nil
}

func (s *RabbitSource) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// notifyLocked wakes Open waiters whenever connection/binding state changes.
func (s *RabbitSource) notifyLocked() {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (s *RabbitSource) reconcile(consumer *messagebus.TopicConsumer) error {
	s.mu.Lock()
	desired := make(map[string]bool, len(s.refs))
	for id := range s.refs {
		desired[id] = true
	}
	bound := make(map[string]bool, len(s.bound))
	for id := range s.bound {
		bound[id] = true
	}
	s.mu.Unlock()
	for id := range desired {
		if bound[id] {
			continue
		}
		if err := consumer.Bind(routingKey(id)); err != nil {
			return err
		}
		s.mu.Lock()
		s.bound[id] = true
		s.notifyLocked()
		s.mu.Unlock()
	}
	for id := range bound {
		if desired[id] {
			continue
		}
		// An Open racing with unbind must wait for the subsequent bind ACK.
		s.mu.Lock()
		delete(s.bound, id)
		s.notifyLocked()
		s.mu.Unlock()
		if err := consumer.Unbind(routingKey(id)); err != nil {
			return err
		}
	}
	return nil
}

func (s *RabbitSource) dispatch(body []byte) {
	var notification struct {
		SessionID string          `json:"session_id"`
		Sequence  *int64          `json:"sequence"`
		Payload   json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(body, &notification) != nil || notification.SessionID == "" || notification.Sequence == nil || *notification.Sequence < 0 {
		return
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(notification.Payload, &object) != nil || object == nil {
		return
	}
	s.local.Publish(notification.SessionID, Event{Sequence: *notification.Sequence, Payload: append([]byte(nil), notification.Payload...)})
}

func (s *RabbitSource) run(ctx context.Context, rawURL, exchange string) {
	defer close(s.done)
	for ctx.Err() == nil {
		consumer, err := messagebus.OpenTopicConsumer(ctx, rawURL, exchange)
		if err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = consumer.Close()
			return
		}
		s.current = consumer
		s.mu.Unlock()
		receiveCtx, cancel := context.WithCancel(ctx)
		receiveDone := make(chan struct{})
		go func() {
			defer close(receiveDone)
			for consumer.Receive(receiveCtx, s.dispatch) == nil {
			}
		}()
		connected := true
		for connected && s.reconcile(consumer) == nil {
			select {
			case <-ctx.Done():
				connected = false
			case <-receiveDone:
				connected = false
			case <-s.wake:
			}
		}
		s.mu.Lock()
		s.current = nil
		clear(s.bound)
		s.notifyLocked()
		s.mu.Unlock()
		cancel()
		_ = consumer.Close()
		<-receiveDone
		// Prevent permanent declaration/authentication failures from busy looping.
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *RabbitSource) Close() error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
		s.notifyLocked()
	}
	consumer := s.current
	s.mu.Unlock()
	if consumer != nil {
		_ = consumer.Close()
	}
	<-s.done
	return s.local.Close()
}
