package workflow

import (
	"context"
	"errors"
	"media_agent/hertz_infra/authn/appcredential"
	d "media_agent/media_agent_gateway/biz/domain/agent_session"
	"testing"
)

type appAuthority struct{ err error }

func (a appAuthority) Verify(context.Context, string, string) (appcredential.Principal, error) {
	return appcredential.Principal{UserID: "user", ApplicationID: "app"}, a.err
}

type appWorkspace struct {
	calls int
	id    string
	err   error
}

func (a *appWorkspace) Check(_ context.Context, _, id string) error {
	a.calls++
	a.id = id
	return a.err
}

type appSessions struct {
	err  error
	user string
}

func (a *appSessions) Authorize(_ context.Context, _, user string) (*d.Session, error) {
	a.user = user
	return &d.Session{WorkspaceID: "bound-workspace"}, a.err
}
func TestApplicationChatRequiresCurrentAuthorityOwnerAndBinding(t *testing.T) {
	ctx := context.Background()
	ws := &appWorkspace{}
	sessions := &appSessions{}
	f := &ApplicationChat{Authority: appAuthority{}, Workspaces: ws, Sessions: sessions}
	if _, e := f.Authorize(ctx, "token", "", ""); !errors.Is(e, appcredential.ErrDenied) {
		t.Fatal("default workspace accepted")
	}
	if _, e := f.Authorize(ctx, "token", "attacker-workspace", "session"); e != nil {
		t.Fatal(e)
	}
	if ws.id != "bound-workspace" || sessions.user != "user" {
		t.Fatal("trusted owner/workspace not used")
	}
	sessions.err = errors.New("foreign session")
	calls := ws.calls
	if _, e := f.Authorize(ctx, "token", "", "session"); !errors.Is(e, appcredential.ErrDenied) || ws.calls != calls {
		t.Fatal("foreign session proceeded")
	}
	sessions.err = nil
	ws.err = appcredential.ErrDenied
	if _, e := f.Authorize(ctx, "token", "", "session"); !errors.Is(e, appcredential.ErrDenied) {
		t.Fatal("cross-app binding accepted")
	}
	f.Authority = appAuthority{appcredential.ErrUnavailable}
	calls = ws.calls
	if _, e := f.Authorize(ctx, "token", "workspace", ""); !errors.Is(e, appcredential.ErrUnavailable) || ws.calls != calls {
		t.Fatal("authority failure fell back")
	}
}
