package workspace

import "context"

type ApplicationBinding struct{ ApplicationID, UserID, TeamID, Name, WorkspaceID, Digest string }
type ApplicationBindings interface {
	LockApplicationKey(context.Context, ApplicationBinding) error
	FindApplicationBinding(context.Context, ApplicationBinding) (ApplicationBinding, error)
	FindApplicationWorkspace(context.Context, ApplicationBinding) (ApplicationBinding, error)
	CreateApplicationBinding(context.Context, ApplicationBinding) error
}
