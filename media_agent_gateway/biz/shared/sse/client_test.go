package sse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestStreamParsesProtocolFrames(t *testing.T) {
	stream := ": keep-alive\r\nid: 7\r\nevent: message\r\ndata: {\"type\":\r\ndata: \"session.started\"}\r\n\r\n"
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader(stream)),
		}, nil
	})}}
	req, _ := http.NewRequest(http.MethodGet, "http://agent.test/chat", nil)
	var events []Event
	if err := client.Stream(context.Background(), req, func(event Event) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "7" || events[0].Type != "message" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if string(events[0].Data) != "{\"type\":\n\"session.started\"}" {
		t.Fatalf("unexpected multiline data: %q", events[0].Data)
	}
}

func TestStreamStopsWhenConsumerFails(t *testing.T) {
	want := errors.New("persist failed")
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data: one\n\ndata: two\n\n"))}, nil
	})}}
	req, _ := http.NewRequest(http.MethodGet, "http://agent.test/chat", nil)
	count := 0
	err := client.Stream(context.Background(), req, func(Event) error {
		count++
		return want
	})
	if !errors.Is(err, want) || count != 1 {
		t.Fatalf("err=%v count=%d", err, count)
	}
}

func TestStreamRejectsNonSSEContentType(t *testing.T) {
	client := &Client{HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	req, _ := http.NewRequest(http.MethodGet, "http://agent.test/chat", nil)
	if err := client.Stream(context.Background(), req, func(Event) error { return nil }); err == nil {
		t.Fatal("expected content type error")
	}
}
