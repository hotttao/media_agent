package client

import (
	"media_agent/hertz_infra/sessioncredential"
	d "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type SessionCredentials struct{ Keys *sessioncredential.Ring }

func (c SessionCredentials) Issue(session *d.Session) (string, string, string, error) {
	token, err := c.Keys.Derive("st1", c.Keys.Active, session.ID, session.UserID, session.WorkspaceID)
	return token, sessioncredential.Hash(token), c.Keys.Active, err
}
func (c SessionCredentials) Reconstruct(session *d.Session) (string, error) {
	token, err := c.Keys.Derive("st1", session.TokenKeyID, session.ID, session.UserID, session.WorkspaceID)
	if err != nil || !sessioncredential.Matches(token, session.TokenHash) {
		return "", d.ErrCredentialUnavailable
	}
	return token, nil
}
