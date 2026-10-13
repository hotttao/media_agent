package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNotFound = errors.New("managed container not found")
var ErrConflict = errors.New("container ownership or revision conflict")
var ErrInvalid = errors.New("invalid container request")

type EngineError struct{ Status int }

func (e EngineError) Error() string { return fmt.Sprintf("Docker Engine status %d", e.Status) }

type Engine struct {
	HTTP *http.Client
	Base string
}

func NewEngine(socket string) *Engine {
	return &Engine{Base: "http://docker/v1.45", HTTP: &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}}
}
func (e *Engine) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.Base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := e.HTTP
	if strings.Contains(path, "/stop?") {
		copy := *e.HTTP
		copy.Timeout = 150 * time.Second
		httpClient = &copy
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Docker Engine unavailable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 404 {
			return ErrNotFound
		}
		return EngineError{resp.StatusCode}
	}
	if out == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}
func (e *Engine) CheckVersion(ctx context.Context) error {
	var version struct {
		APIVersion string `json:"ApiVersion"`
		Min        string `json:"MinAPIVersion"`
	}
	base := e.Base
	e.Base = strings.TrimSuffix(base, "/v1.45")
	err := e.call(ctx, "GET", "/version", nil, &version)
	e.Base = base
	if err != nil {
		return err
	}
	parse := func(s string) int {
		p := strings.Split(s, ".")
		if len(p) != 2 {
			return 0
		}
		a, _ := strconv.Atoi(p[0])
		b, _ := strconv.Atoi(p[1])
		return a*100 + b
	}
	if parse(version.APIVersion) < 141 || version.Min != "" && parse(version.Min) > 145 {
		return ErrInvalid
	}
	selected := 145
	if server := parse(version.APIVersion); server < selected {
		selected = server
	}
	e.Base = strings.TrimSuffix(base, "/v1.45") + fmt.Sprintf("/v%d.%d", selected/100, selected%100)
	return nil
}

type Inspection struct {
	ID     string `json:"Id"`
	Name   string
	Config struct {
		Image  string
		User   string
		Labels map[string]string
		Env    []string
	}
	State struct {
		Running    bool
		Restarting bool
		StartedAt  string
		FinishedAt string
		Health     *struct{ Status string }
	}
	Mounts []struct {
		Type, Source, Destination string
		RW                        bool
	}
	HostConfig struct {
		Privileged    bool
		NetworkMode   string
		RestartPolicy struct{ Name string }
		Binds         []string
	}
	NetworkSettings struct{ Networks map[string]any }
}

func (e *Engine) Inspect(ctx context.Context, name string) (*Inspection, error) {
	var out Inspection
	err := e.call(ctx, "GET", "/containers/"+url.PathEscape(name)+"/json", nil, &out)
	return &out, err
}
func (e *Engine) Create(ctx context.Context, name string, body any) error {
	var out struct{ ID string }
	return e.call(ctx, "POST", "/containers/create?name="+url.QueryEscape(name), body, &out)
}
func (e *Engine) Start(ctx context.Context, id string) error {
	return e.call(ctx, "POST", "/containers/"+url.PathEscape(id)+"/start", nil, nil)
}
func (e *Engine) Stop(ctx context.Context, id string) error {
	return e.call(ctx, "POST", "/containers/"+url.PathEscape(id)+"/stop?t=120", nil, nil)
}
