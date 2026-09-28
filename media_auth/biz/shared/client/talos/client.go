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
	"strings"
	"time"
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
		return VerifiedKey{}, fmt.Errorf("%w: %s", ErrInvalidCredential, response.ErrorCode)
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
	var response struct {
		IssuedAPIKey issuedAPIKey `json:"issued_api_key"`
		Secret       string       `json:"secret"`
	}
	if err := c.do(ctx, http.MethodPost, apiPrefix+"/issuedApiKeys/"+url.PathEscape(keyID)+":rotate", map[string]any{"scopes": scopes}, &response); err != nil {
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
			return VerifiedKey{}, fmt.Errorf("decode talos expiry: %w", err)
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
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.adminToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.adminToken)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("talos %s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if output == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output); err != nil {
		return fmt.Errorf("decode talos response: %w", err)
	}
	return nil
}
