package talos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

const apiPrefix = "/v2alpha1/admin"

var (
	ErrInvalidCredential = errors.New("talos credential is invalid")
	ErrUnavailable       = errors.New("talos is unavailable")
)

type Config struct {
	BaseURL    string
	AdminToken string
	Timeout    time.Duration
	Transport  http.RoundTripper
}

type Client struct {
	baseURL    string
	adminToken string
	http       *http.Client
}

type IssueCommand struct {
	Name, ActorID, RequestID, TTL string
	Scopes                        []string
	Metadata                      map[string]any
}

type IssuedKey struct {
	KeyID, ActorID, Secret, Status string
	Scopes                         []string
	ExpiresAt                      time.Time
}

type VerifiedKey struct {
	KeyID, ActorID, Status string
	Scopes                 []string
	Metadata               map[string]any
	ExpiresAt              time.Time
}

type KeyMetadata = VerifiedKey

type KeyPage struct {
	Keys          []KeyMetadata
	NextPageToken string
}

func (c *Client) List(ctx context.Context, actorID, pageToken string) (KeyPage, error) {
	if strings.TrimSpace(actorID) == "" {
		return KeyPage{}, errors.New("talos list requires actor id")
	}
	query := url.Values{"filter": {"actor_id=" + strconv.Quote(actorID)}, "page_size": {"100"}}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var response struct {
		Keys          []issuedAPIKey `json:"issued_api_keys"`
		NextPageToken string         `json:"next_page_token"`
	}
	if err := c.do(ctx, http.MethodGet, apiPrefix+"/issuedApiKeys?"+query.Encode(), nil, &response); err != nil {
		return KeyPage{}, err
	}
	page := KeyPage{Keys: make([]KeyMetadata, 0, len(response.Keys)), NextPageToken: response.NextPageToken}
	for _, key := range response.Keys {
		metadata, err := key.toVerified()
		if err != nil || metadata.KeyID == "" || metadata.ActorID != actorID {
			return KeyPage{}, errors.New("talos returned invalid key list")
		}
		page.Keys = append(page.Keys, metadata)
	}
	return page, nil
}

func New(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("talos base URL is required")
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid talos base URL: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	transport := cfg.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{baseURL: baseURL, adminToken: cfg.AdminToken, http: &http.Client{Timeout: timeout, Transport: transport}}, nil
}

func (c *Client) Issue(ctx context.Context, cmd IssueCommand) (IssuedKey, error) {
	if strings.TrimSpace(cmd.Name) == "" || strings.TrimSpace(cmd.ActorID) == "" {
		return IssuedKey{}, fmt.Errorf("talos issue requires name and actor id")
	}
	body := map[string]any{"name": cmd.Name, "actor_id": cmd.ActorID, "scopes": cmd.Scopes}
	if cmd.RequestID != "" {
		body["request_id"] = cmd.RequestID
	}
	if cmd.TTL != "" {
		body["ttl"] = cmd.TTL
	}
	if cmd.Metadata != nil {
		body["metadata"] = cmd.Metadata
	}
	var response struct {
		IssuedAPIKey issuedAPIKey `json:"issued_api_key"`
		Secret       string       `json:"secret"`
	}
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/issuedApiKeys", body, &response); err != nil {
		return IssuedKey{}, err
	}
	if response.IssuedAPIKey.KeyID == "" || response.Secret == "" {
		return IssuedKey{}, fmt.Errorf("talos issue returned incomplete key")
	}
	return response.IssuedAPIKey.toIssued(response.Secret)
}

func (c *Client) Verify(ctx context.Context, secret string) (VerifiedKey, error) {
	if strings.TrimSpace(secret) == "" {
		return VerifiedKey{}, ErrInvalidCredential
	}
	var response verifyResponse
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/apiKeys:verify", map[string]string{"credential": secret}, &response); err != nil {
		return VerifiedKey{}, err
	}
	if !response.IsValid {
		// Upstream error fields may echo credentials; only expose our sentinel.
		return VerifiedKey{}, ErrInvalidCredential
	}
	return response.toVerified()
}

func (c *Client) Get(ctx context.Context, keyID string) (KeyMetadata, error) {
	var response issuedAPIKey
	if err := c.do(ctx, http.MethodGet, apiPrefix+"/issuedApiKeys/"+url.PathEscape(keyID), nil, &response); err != nil {
		return KeyMetadata{}, err
	}
	return response.toVerified()
}

func (c *Client) Revoke(ctx context.Context, keyID string) error {
	return c.do(ctx, http.MethodPost, apiPrefix+"/issuedApiKeys/"+url.PathEscape(keyID)+":revoke", map[string]string{
		"reason":      "REVOCATION_REASON_PRIVILEGE_WITHDRAWN",
		"description": "revoked by media_auth",
	}, nil)
}

func (c *Client) Rotate(ctx context.Context, keyID string, scopes []string) (IssuedKey, error) {
	return c.RotateWithMetadata(ctx, keyID, scopes, nil)
}

func (c *Client) RotateWithMetadata(ctx context.Context, keyID string, scopes []string, metadata map[string]any) (IssuedKey, error) {
	var response struct {
		IssuedAPIKey issuedAPIKey `json:"issued_api_key"`
		Secret       string       `json:"secret"`
	}
	body := map[string]any{"scopes": scopes}
	if metadata != nil {
		body["metadata"] = metadata
	}
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/issuedApiKeys/"+url.PathEscape(keyID)+":rotate", body, &response); err != nil {
		return IssuedKey{}, err
	}
	return response.IssuedAPIKey.toIssued(response.Secret)
}

type issuedAPIKey struct {
	KeyID      string         `json:"key_id"`
	ActorID    string         `json:"actor_id"`
	Status     string         `json:"status"`
	Scopes     []string       `json:"scopes"`
	Metadata   map[string]any `json:"metadata"`
	ExpireTime string         `json:"expire_time"`
}

type verifyResponse struct {
	issuedAPIKey
	IsValid      bool   `json:"is_valid"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (k issuedAPIKey) toIssued(secret string) (IssuedKey, error) {
	if k.KeyID == "" || secret == "" {
		return IssuedKey{}, errors.New("talos returned incomplete issued key")
	}
	verified, err := k.toVerified()
	if err != nil {
		return IssuedKey{}, err
	}
	return IssuedKey{KeyID: verified.KeyID, ActorID: verified.ActorID, Secret: secret, Status: verified.Status, Scopes: verified.Scopes, ExpiresAt: verified.ExpiresAt}, nil
}

func (k issuedAPIKey) toVerified() (VerifiedKey, error) {
	var expiresAt time.Time
	var err error
	if k.ExpireTime != "" {
		expiresAt, err = time.Parse(time.RFC3339Nano, k.ExpireTime)
		if err != nil {
			return VerifiedKey{}, errors.New("talos returned invalid expiry")
		}
	}
	return VerifiedKey{KeyID: k.KeyID, ActorID: k.ActorID, Status: k.Status, Scopes: append([]string(nil), k.Scopes...), Metadata: k.Metadata, ExpiresAt: expiresAt}, nil
}

func (r verifyResponse) toVerified() (VerifiedKey, error) { return r.issuedAPIKey.toVerified() }

func (c *Client) do(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.adminToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.adminToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(ErrUnavailable, ctx.Err())
		}
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Never surface an upstream response body: HTTP failures can contain
		// echoed Secret values and are later logged by handlers/compensators.
		if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
		}
		return fmt.Errorf("talos returned HTTP %d", response.StatusCode)
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return errors.New("talos returned invalid JSON")
	}
	return nil
}
