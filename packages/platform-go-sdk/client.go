// Package platformsdk is the server-side Go SDK for application access.
package platformsdk

import (
	"context"
	"encoding/base64"
	"errors"
	adapter "media_agent/packages/platform-go-sdk/biz/shared/client"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	"media_agent/packages/platform-go-sdk/biz/shared/credential"
	flow "media_agent/packages/platform-go-sdk/biz/workflow/application_access"
	"net/url"
	"strings"
)

type Client struct {
	api   *adapter.Clients
	cache *credential.Cache
	basic string
}
type UserClient = flow.UserClient
type Error = contract.Error
type Scope = string

const (
	WorkspaceRead   = "workspace.read"
	WorkspaceEnsure = "workspace.ensure"
	SessionChat     = "session.chat"
)

func New(c Config) (*Client, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || c.ClientID == "" || strings.ContainsAny(c.ClientID, ":\r\n") || c.ClientSecret == "" {
		return nil, errors.New("invalid SDK configuration")
	}
	api, e := adapter.New(strings.TrimRight(u.String(), "/"))
	if e != nil {
		return nil, e
	}
	return &Client{api, credential.NewCache(), "Basic " + base64.StdEncoding.EncodeToString([]byte(c.ClientID+":"+c.ClientSecret))}, nil
}
func (c *Client) As(externalID, teamID string, scopes ...Scope) (*UserClient, error) {
	return flow.New(c.api, c.cache, c.basic, externalID, teamID, scopes)
}
func (c *Client) UpsertUser(ctx context.Context, id string, input UserProfileInput) (UserProfile, error) {
	if _, e := c.As(id, ""); e != nil {
		return UserProfile{}, e
	}
	return c.api.UpsertUser(ctx, c.basic, id, input)
}
func (c *Client) GetUser(ctx context.Context, id string) (UserProfile, error) {
	if _, e := c.As(id, ""); e != nil {
		return UserProfile{}, e
	}
	return c.api.GetUser(ctx, c.basic, id)
}
