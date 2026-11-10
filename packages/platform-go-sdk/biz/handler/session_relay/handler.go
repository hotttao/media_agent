// Package session_relay adapts an application's authenticated HTTP session to platform chat.
package session_relay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	flow "media_agent/packages/platform-go-sdk/biz/workflow/session_relay"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	Prefix string
	// Origin must be the application's configured public origin, never a forwarded header.
	Origin       string
	Authenticate func(*http.Request) (context.Context, error)
	Resolve      func(context.Context, string) (*flow.Flow, string, error)
}
type Handler struct{ config Config }

func New(c Config) (*Handler, error) {
	u, e := url.Parse(c.Origin)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || c.Authenticate == nil || c.Resolve == nil {
		return nil, errors.New("invalid relay configuration")
	}
	c.Prefix = strings.TrimRight(c.Prefix, "/")
	if c.Prefix == "" || !strings.HasPrefix(c.Prefix, "/") || strings.ContainsAny(c.Prefix, "%?#\\") {
		return nil, errors.New("invalid relay prefix")
	}
	return &Handler{c}, nil
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var p *contract.Error
	if errors.As(err, &p) && p.StatusCode >= 400 && p.StatusCode < 500 {
		status = p.StatusCode
	}
	reply(w, status, map[string]string{"error": http.StatusText(status)})
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, h.config.Prefix+"/") {
		http.NotFound(w, r)
		return
	}
	if strings.ContainsAny(strings.ToLower(r.URL.EscapedPath()), "\\") || strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%2f") || strings.Contains(strings.ToLower(r.URL.EscapedPath()), "%5c") {
		reply(w, 400, map[string]string{"error": "invalid path"})
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, h.config.Prefix+"/"), "/")
	if len(parts) < 3 || parts[0] != "workspaces" || parts[2] != "sessions" || contract.ValidateSegments(parts...) != nil {
		http.NotFound(w, r)
		return
	}
	action := ""
	method := "GET"
	switch {
	case len(parts) == 3:
		if r.Method == "POST" {
			action = "create"
			method = "POST"
		} else {
			action = "sessions"
		}
	case len(parts) == 5:
		action = parts[4]
		switch action {
		case "messages", "events", "requests":
		case "turns", "interrupt":
			method = "POST"
		default:
			http.NotFound(w, r)
			return
		}
	case len(parts) == 9 && parts[4] == "turns" && parts[6] == "requests" && parts[8] == "resolve":
		action = "resolve"
		method = "POST"
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		reply(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	if method == "POST" && r.Header.Get("Origin") != h.config.Origin {
		reply(w, 403, map[string]string{"error": "origin rejected"})
		return
	}
	ctx, err := h.config.Authenticate(r)
	if err != nil || ctx == nil {
		reply(w, 401, map[string]string{"error": "unauthorized"})
		return
	}
	// Preserve application identity and guarantee disconnect cancellation even if the callback returns a detached context.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(r.Context(), cancel)
	defer stop()
	client, workspace, err := h.config.Resolve(ctx, parts[1])
	if err != nil || client == nil || workspace == "" {
		reply(w, 404, map[string]string{"error": "workspace not found"})
		return
	}
	sid := ""
	if len(parts) > 3 {
		sid = parts[3]
		if err = client.RequireSession(ctx, workspace, sid); err != nil {
			failure(w, err)
			return
		}
	}
	var body struct {
		Title          string `json:"title"`
		Message        string `json:"message"`
		IdempotencyKey string `json:"idempotency_key"`
		ResponseJSON   string `json:"response_json"`
	}
	if method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err = dec.Decode(&body); err != nil {
			reply(w, 400, map[string]string{"error": "invalid JSON body"})
			return
		}
		if dec.Decode(new(any)) != io.EOF {
			reply(w, 400, map[string]string{"error": "invalid JSON body"})
			return
		}
	}
	var value any
	switch action {
	case "sessions":
		var rows []contract.Session
		rows, err = client.ListSessions(ctx, workspace)
		value = map[string]any{"sessions": rows}
	case "create":
		value, err = client.CreateSession(ctx, workspace, body.Title)
	case "messages":
		var rows []contract.Message
		rows, err = client.ListMessages(ctx, sid)
		value = map[string]any{"messages": rows}
	case "requests":
		var rows []contract.AgentRequest
		rows, err = client.ListRequests(ctx, sid)
		value = map[string]any{"requests": rows}
	case "turns":
		if strings.TrimSpace(body.Message) == "" || strings.TrimSpace(body.IdempotencyKey) == "" {
			reply(w, 400, map[string]string{"error": "message and idempotency_key required"})
			return
		}
		value, err = client.RunTurn(ctx, sid, body.Message, body.IdempotencyKey)
	case "interrupt":
		value, err = client.Interrupt(ctx, sid)
	case "resolve":
		if !json.Valid([]byte(body.ResponseJSON)) {
			reply(w, 400, map[string]string{"error": "invalid response_json"})
			return
		}
		value, err = client.ResolveRequest(ctx, sid, parts[5], parts[7], body.ResponseJSON)
	case "events":
		cursor := r.URL.Query().Get("after_sequence")
		if cursor == "" {
			cursor = r.Header.Get("Last-Event-ID")
		}
		after := int64(0)
		if cursor != "" {
			after, err = strconv.ParseInt(cursor, 10, 64)
			if err != nil || after < 0 {
				reply(w, 400, map[string]string{"error": "invalid event cursor"})
				return
			}
		}
		var upstream *http.Response
		upstream, err = client.Events(ctx, sid, after)
		if err != nil {
			failure(w, err)
			return
		}
		defer upstream.Body.Close()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		controller := http.NewResponseController(w)
		if err = controller.Flush(); err != nil {
			return
		}
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := upstream.Body.Read(buffer)
			if n > 0 {
				if _, err = w.Write(buffer[:n]); err != nil {
					return
				}
				if err = controller.Flush(); err != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	if err != nil {
		failure(w, err)
		return
	}
	reply(w, 200, value)
}
