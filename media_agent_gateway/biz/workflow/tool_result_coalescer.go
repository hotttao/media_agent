package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var errCoalescerClosed = errors.New("tool result coalescer is closed")

type pendingToolResult struct {
	latest      json.RawMessage
	lastWritten json.RawMessage
	dirty       bool
	timer       *time.Timer
}

type toolResultCoalescer struct {
	mu       sync.Mutex
	window   time.Duration
	write    func(json.RawMessage) error
	entries  map[string]*pendingToolResult
	closed   bool
	err      error
	inflight sync.WaitGroup
}

func newToolResultCoalescer(window time.Duration, write func(json.RawMessage) error) *toolResultCoalescer {
	return &toolResultCoalescer{window: window, write: write, entries: make(map[string]*pendingToolResult)}
}

func (c *toolResultCoalescer) Accept(key string, raw json.RawMessage) error {
	copyRaw := append(json.RawMessage(nil), raw...)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errCoalescerClosed
	}
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	entry := c.entries[key]
	if entry == nil {
		entry = &pendingToolResult{}
		c.entries[key] = entry
		if err := c.write(copyRaw); err != nil {
			if c.err == nil {
				c.err = err
			}
			c.mu.Unlock()
			return err
		}
		entry.lastWritten = copyRaw
		c.mu.Unlock()
		return nil
	}
	if bytes.Equal(entry.lastWritten, copyRaw) || bytes.Equal(entry.latest, copyRaw) {
		c.mu.Unlock()
		return nil
	}
	entry.latest = copyRaw
	entry.dirty = true
	if entry.timer == nil {
		c.scheduleLocked(key, entry)
	}
	c.mu.Unlock()
	return nil
}

func (c *toolResultCoalescer) scheduleLocked(key string, entry *pendingToolResult) {
	delay := c.window
	if delay <= 0 {
		delay = time.Millisecond
	}
	c.inflight.Add(1)
	entry.timer = time.AfterFunc(delay, func() {
		defer c.inflight.Done()
		c.flushKey(key)
	})
}

func (c *toolResultCoalescer) flushKey(key string) {
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		c.mu.Unlock()
		return
	}
	entry.timer = nil
	if c.closed || !entry.dirty || c.err != nil {
		c.mu.Unlock()
		return
	}
	raw := append(json.RawMessage(nil), entry.latest...)
	entry.dirty = false
	c.mu.Unlock()

	if err := c.write(raw); err != nil {
		c.recordError(err)
		return
	}
	c.mu.Lock()
	entry.lastWritten = raw
	c.mu.Unlock()
}

func (c *toolResultCoalescer) Flush() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		for _, entry := range c.entries {
			if entry.timer != nil && entry.timer.Stop() {
				c.inflight.Done()
			}
			entry.timer = nil
		}
	}
	c.mu.Unlock()
	c.inflight.Wait()

	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	pending := make([]json.RawMessage, 0, len(c.entries))
	for _, entry := range c.entries {
		if entry.dirty && !bytes.Equal(entry.lastWritten, entry.latest) {
			pending = append(pending, append(json.RawMessage(nil), entry.latest...))
			entry.dirty = false
		}
	}
	c.mu.Unlock()
	for _, raw := range pending {
		if err := c.write(raw); err != nil {
			c.recordError(err)
			return err
		}
	}
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}

func (c *toolResultCoalescer) recordError(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
}
