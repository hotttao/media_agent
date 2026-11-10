package session_relay

import (
	"context"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
)

// RequireSession checks the server-selected workspace before any session operation.
// The platform still enforces authorization on every request.
func (u *Flow) RequireSession(ctx context.Context, workspaceID, sessionID string) error {
	if err := contract.ValidateSegments(workspaceID, sessionID); err != nil {
		return err
	}
	sessions, err := u.ListSessions(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.ID == sessionID && s.WorkspaceID == workspaceID {
			return nil
		}
	}
	return &contract.Error{StatusCode: 404}
}
