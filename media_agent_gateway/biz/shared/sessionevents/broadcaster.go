package sessionevents

import "sync"

const defaultBuffer = 128

type Event struct {
	// Sequence is zero for a transient live frame. Durable events always have
	// a positive sequence and can advance a subscriber's replay cursor.
	Sequence int64
	Payload  []byte
}

type Subscription struct {
	Events <-chan Event
	cancel func()
}

func (s Subscription) Close() { s.cancel() }

type Broadcaster struct {
	mu          sync.Mutex
	nextID      uint64
	buffer      int
	subscribers map[string]map[uint64]chan Event
}

func New(buffer int) *Broadcaster {
	if buffer <= 0 {
		buffer = defaultBuffer
	}
	return &Broadcaster{buffer: buffer, subscribers: make(map[string]map[uint64]chan Event)}
}

func (b *Broadcaster) Subscribe(sessionID string) Subscription {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	ch := make(chan Event, b.buffer)
	if b.subscribers[sessionID] == nil {
		b.subscribers[sessionID] = make(map[uint64]chan Event)
	}
	b.subscribers[sessionID][id] = ch
	b.mu.Unlock()
	var once sync.Once
	return Subscription{Events: ch, cancel: func() {
		once.Do(func() { b.remove(sessionID, id) })
	}}
}

func (b *Broadcaster) Publish(sessionID string, event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subscribers[sessionID] {
		select {
		case ch <- event:
		default:
			close(ch)
			delete(b.subscribers[sessionID], id)
		}
	}
	if len(b.subscribers[sessionID]) == 0 {
		delete(b.subscribers, sessionID)
	}
}

func (b *Broadcaster) remove(sessionID string, id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	group := b.subscribers[sessionID]
	if ch, ok := group[id]; ok {
		close(ch)
		delete(group, id)
	}
	if len(group) == 0 {
		delete(b.subscribers, sessionID)
	}
}
