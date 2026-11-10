// Package wx_channel adapts the local wx_channel MCP service. WeChat credentials
// and download/decryption remain owned by wx_channel.
package wx_channel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"media_agent/media_worker/biz/shared/worker_runtime"
)

var ErrUnavailable = errors.New("wx_channel unavailable; check local MCP service and token")
var ErrDownload = errors.New("wx_channel download failed; inspect the stored record in wx_channel")

type Client struct {
	Session   *mcp.ClientSession
	Downloads string
}
type bearer struct {
	token string
	base  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

func Open(ctx context.Context, endpoint, tokenFile, downloads string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/mcp" || !(u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()) {
		return nil, ErrUnavailable
	}
	raw, err := os.ReadFile(tokenFile)
	if err != nil || len(raw) > 4096 || strings.TrimSpace(string(raw)) == "" {
		return nil, ErrUnavailable
	}
	// Never forward local authentication through a redirect or an environment proxy.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	httpClient := &http.Client{Transport: bearer{strings.TrimSpace(string(raw)), transport}, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "media-worker-wx-channel", Version: "1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Client{Session: session, Downloads: downloads}, nil
}
func (c *Client) Close() error { return c.Session.Close() }
func (c *Client) Call(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	r, err := c.Session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || r == nil || r.IsError {
		return nil, ErrUnavailable
	}
	raw, err := json.Marshal(r.StructuredContent)
	if err != nil {
		return nil, ErrUnavailable
	}
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Data == nil {
		return nil, ErrUnavailable
	}
	return envelope.Data, nil
}

// Download returns only a completed, privately snapshotted artifact. Repeated
// calls use wx_channel's record identity and completed-download deduplication.
func (c *Client) Download(ctx context.Context, recordID, target string) (string, error) {
	// A completed local snapshot is immutable for this Dispatch/Attempt. Upload
	// retries and process restarts must use these bytes, not a changed source.
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 256<<20 {
			return "", ErrDownload
		}
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", ErrDownload
	}
	// A failed persisted download needs an explicit user retry in wx_channel;
	// polling this workflow must not start an unbounded series of downloads.
	state, stateErr := c.Call(ctx, "get_local_video", map[string]any{"id": recordID, "source": "download"})
	if stateErr == nil && state["status"] == "failed" {
		return "", ErrDownload
	}
	data, err := c.Call(ctx, "download_video_for_analysis", map[string]any{"record_id": recordID})
	if err != nil {
		return "", err
	}
	if data["status"] == "failed" {
		return "", ErrDownload
	}
	if data["status"] != "completed" {
		return "", worker_runtime.ErrExecutionPending
	}
	file, ok := data["file"].(map[string]any)
	if !ok || file["exists"] != true {
		return "", ErrDownload
	}
	rel, ok := file["relative_path"].(string)
	if !ok || !filepath.IsLocal(rel) || strings.ContainsAny(rel, "\\\x00:") {
		return "", ErrDownload
	}
	root, err := os.OpenRoot(c.Downloads)
	if err != nil {
		return "", ErrDownload
	}
	defer root.Close()
	source, err := root.Open(rel)
	if err != nil {
		return "", ErrDownload
	}
	defer source.Close()
	header := make([]byte, 12)
	if _, err := source.ReadAt(header, 0); err != nil || string(header[4:8]) != "ftyp" {
		return "", ErrDownload
	}
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 256<<20 {
		return "", ErrDownload
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return "", ErrDownload
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".reference-")
	if err != nil {
		return "", ErrDownload
	}
	defer os.Remove(temp.Name())
	n, copyErr := io.Copy(temp, io.LimitReader(source, (256<<20)+1))
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || n != info.Size() || ctx.Err() != nil {
		return "", ErrDownload
	}
	if err = os.Rename(temp.Name(), target); err != nil {
		return "", ErrDownload
	}
	return target, nil
}
