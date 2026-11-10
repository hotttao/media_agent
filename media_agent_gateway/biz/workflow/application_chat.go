package workflow

import (
	"context"
	"media_agent/hertz_infra/authn/appcredential"
	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type ApplicationAuthority interface {
	Verify(context.Context, string, string) (appcredential.Principal, error)
}
type ApplicationWorkspace interface {
	Check(context.Context, string, string) error
}
type ApplicationSessions interface {
	Authorize(context.Context, string, string) (*domain.Session, error)
}

// ApplicationChat establishes the application boundary before existing chat use cases execute.
type ApplicationChat struct {
	Authority  ApplicationAuthority
	Workspaces ApplicationWorkspace
	Sessions   ApplicationSessions
}

func (f *ApplicationChat) Authorize(ctx context.Context, token, workspace, session string) (appcredential.Principal, error) {
	p, e := f.Authority.Verify(ctx, token, "session.chat")
	if e != nil {
		return p, e
	}
	if session != "" {
		s, e := f.Sessions.Authorize(ctx, session, p.UserID)
		if e != nil {
			return p, appcredential.ErrDenied
		}
		workspace = s.WorkspaceID
	}
	if workspace == "" {
		return p, appcredential.ErrDenied
	}
	if e = f.Workspaces.Check(ctx, token, workspace); e != nil {
		return p, e
	}
	return p, nil
}
