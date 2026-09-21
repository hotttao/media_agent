package media_storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

var (
	ErrConflict    = errors.New("stored object conflict")
	ErrUnavailable = errors.New("media storage unavailable")
)

type PutInput struct {
	ObjectPath  string
	ContentType string
	SizeBytes   int64
	Body        io.Reader
}

type ObjectInfo struct {
	ObjectPath  string `json:"object_path"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	Checksum    string `json:"checksum"`
}

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: timeout}}
}

func (c *Client) Put(ctx context.Context, input PutInput) (ObjectInfo, error) {
	if c == nil || c.baseURL == "" || c.token == "" {
		return ObjectInfo{}, ErrUnavailable
	}
	u, err := url.Parse(c.baseURL + "/api/internal/v1/objects")
	if err != nil {
		return ObjectInfo{}, ErrUnavailable
	}
	query := u.Query()
	query.Set("object_path", input.ObjectPath)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), input.Body)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("build storage request: %w", err)
	}
	req.Header.Set("Content-Type", input.ContentType)
	req.Header.Set("X-Media-Storage-Internal-Token", c.token)
	if input.SizeBytes >= 0 {
		req.ContentLength = input.SizeBytes
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	resp, err := c.http.Do(req)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return ObjectInfo{}, ErrConflict
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return ObjectInfo{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	var result ObjectInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return ObjectInfo{}, fmt.Errorf("%w: decode response: %v", ErrUnavailable, err)
	}
	return result, nil
}
