// Package sse provides the shared transport for consuming Agent Runtime SSE streams.
package sse

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const defaultMaxFrameBytes = 2 * 1024 * 1024

type Event struct {
	ID   string
	Type string
	Data []byte
}

type Consumer func(Event) error

type Client struct {
	HTTP          *http.Client
	MaxFrameBytes int
}

// Stream executes req and consumes each SSE frame as soon as it is complete.
// It owns protocol framing only; Agent-specific request and event semantics
// remain in the caller.
func (c *Client) Stream(ctx context.Context, req *http.Request, consume Consumer) error {
	req = req.Clone(ctx)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sse: request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("sse: upstream status %d: %s", resp.StatusCode, body)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || mediaType != "text/event-stream" {
			return fmt.Errorf("sse: unexpected content type %q", contentType)
		}
	}
	return c.consume(resp.Body, consume)
}

func (c *Client) consume(body io.Reader, consume Consumer) error {
	maxFrameBytes := c.MaxFrameBytes
	if maxFrameBytes <= 0 {
		maxFrameBytes = defaultMaxFrameBytes
	}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), maxFrameBytes)
	event := Event{}
	data := make([]string, 0, 1)
	flush := func() error {
		if len(data) == 0 {
			event = Event{}
			return nil
		}
		event.Data = []byte(strings.Join(data, "\n"))
		if err := consume(event); err != nil {
			return err
		}
		event = Event{}
		data = data[:0]
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			data = append(data, value)
		case "event":
			event.Type = value
		case "id":
			event.ID = value
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse: read stream: %w", err)
	}
	return flush()
}
