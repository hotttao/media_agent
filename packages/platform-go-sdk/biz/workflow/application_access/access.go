package application_access

import (
	"context"

	"encoding/json"
	"errors"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
	agent "media_agent/packages/platform-go-sdk/biz/workflow/agent_access"
	session "media_agent/packages/platform-go-sdk/biz/workflow/session_relay"
	"sort"

	"media_agent/packages/platform-go-sdk/biz/shared/credential"
)

type AgentAccess = agent.Flow
type SessionAccess = session.Flow
type UserClient struct {
	*AgentAccess
	*SessionAccess
	api                            API
	cache                          *credential.Cache
	basic, externalID, teamID, key string
	scopes                         []string
}

func New(api API, cache *credential.Cache, basic, externalID, teamID string, scopes []string) (*UserClient, error) {
	if err := contract.ValidateSegments(externalID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	ordered := []string{}
	for _, v := range scopes {
		if v != "workspace.read" && v != "workspace.ensure" && v != "session.chat" {
			return nil, errors.New("invalid scope")
		}
		if !seen[v] {
			ordered = append(ordered, v)
			seen[v] = true
		}
	}
	if (teamID == "") != (len(ordered) == 0) {
		return nil, errors.New("team and scopes required together")
	}
	sort.Strings(ordered)
	key, _ := json.Marshal([]any{externalID, teamID, ordered})
	u := &UserClient{api: api, cache: cache, basic: basic, externalID: externalID, teamID: teamID, key: string(key), scopes: ordered}
	u.AgentAccess = agent.New(api, u.token, u.invalidate)
	u.SessionAccess = session.New(api, u.token, u.invalidate)
	return u, nil
}
func (u *UserClient) invalidate(e error) {
	var p *contract.Error
	if errors.As(e, &p) && (p.StatusCode == 401 || p.StatusCode == 403) {
		u.cache.Invalidate(u.key)
	}
}
func (u *UserClient) token(ctx context.Context) (string, error) {
	return u.cache.Get(ctx, u.key, func(ctx context.Context) (contract.Token, error) {
		return u.api.Exchange(ctx, u.basic, u.externalID, u.teamID, u.scopes)
	})
}
