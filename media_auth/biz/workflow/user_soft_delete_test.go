package workflow

import (
	"context"
	"errors"
	"testing"

	domainauth "media_agent/media_auth/biz/domain/auth"
)

type deleteUsersFake struct {
	users        map[string]domainauth.User
	activeAdmins int
}

func (f *deleteUsersFake) FindByID(_ context.Context, id string) (domainauth.User, error) {
	u, ok := f.users[id]
	if !ok {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	return u, nil
}
func (f *deleteUsersFake) CountActiveAdmins(context.Context) (int, error) { return f.activeAdmins, nil }
func (f *deleteUsersFake) SoftDeleteUser(_ context.Context, id, by string) (domainauth.User, error) {
	u, ok := f.users[id]
	if !ok || u.Status == domainauth.UserStatusDeleted {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	u.Status, u.AuthVersion, u.DeletedBy = domainauth.UserStatusDeleted, u.AuthVersion+1, by
	f.users[id] = u
	return u, nil
}
func (f *deleteUsersFake) UpdateStatus(_ context.Context, id, from, to string) (domainauth.User, error) {
	u, ok := f.users[id]
	if !ok || u.Status != from {
		return domainauth.User{}, domainauth.ErrUserNotFound
	}
	u.Status, u.AuthVersion = to, u.AuthVersion+1
	f.users[id] = u
	return u, nil
}

type outboxFake struct {
	events    []domainauth.AccessRevocationEvent
	delivered bool
	failed    bool
}

func (f *outboxFake) Enqueue(_ context.Context, e domainauth.AccessRevocationEvent) error {
	f.events = append(f.events, e)
	return nil
}
func (f *outboxFake) ClaimPending(context.Context, int) ([]domainauth.AccessRevocationEvent, error) {
	return f.events, nil
}
func (f *outboxFake) MarkDelivered(context.Context, string) error {
	f.delivered = true
	f.events = nil
	return nil
}
func (f *outboxFake) MarkFailed(context.Context, string, string) error { f.failed = true; return nil }

type txFake struct{}

func (txFake) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type revokeFake struct {
	calls int
	err   error
}

type sessionRevokeFake struct {
	calls      int
	identityID string
	err        error
}

func (f *sessionRevokeFake) RevokeIdentitySessions(_ context.Context, identityID string) error {
	f.calls++
	f.identityID = identityID
	return f.err
}

func (f *revokeFake) RevokeAllForUser(context.Context, string) error      { f.calls++; return f.err }
func (f *revokeFake) RevokeUserCredentials(context.Context, string) error { f.calls++; return f.err }
func (f *revokeFake) DeleteTokensForUser(context.Context, string) error   { f.calls++; return f.err }

func newDeleteFlow(users *deleteUsersFake, outbox *outboxFake, token, oauth, mcp *revokeFake) *UserSoftDelete {
	return NewUserSoftDelete(users, outbox, token, oauth, mcp, txFake{})
}

func TestUserSoftDeleteCommitsTerminalStateAndEvent(t *testing.T) {
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"u1": {ID: "u1", Role: "admin", Status: "active", AuthVersion: 1},
		"u2": {ID: "u2", Role: "user", Status: "active", AuthVersion: 4},
	}, activeAdmins: 1}
	outbox := &outboxFake{}
	flow := newDeleteFlow(users, outbox, &revokeFake{}, &revokeFake{}, &revokeFake{})
	if err := flow.Delete(context.Background(), "u1", "u2"); err != nil {
		t.Fatal(err)
	}
	if users.users["u2"].Status != "deleted" || users.users["u2"].AuthVersion != 5 {
		t.Fatalf("user=%+v", users.users["u2"])
	}
	if len(outbox.events) != 1 || outbox.events[0].AuthVersion != 5 {
		t.Fatalf("events=%+v", outbox.events)
	}
}

func TestUserSoftDeleteProtectsSelfAndLastAdmin(t *testing.T) {
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"a1": {ID: "a1", Role: "admin", Status: "active", AuthVersion: 1},
		"a2": {ID: "a2", Role: "admin", Status: "active", AuthVersion: 1},
	}, activeAdmins: 1}
	flow := newDeleteFlow(users, &outboxFake{}, &revokeFake{}, &revokeFake{}, &revokeFake{})
	if err := flow.Delete(context.Background(), "a1", "a1"); err == nil {
		t.Fatal("self delete should fail")
	}
	if err := flow.Delete(context.Background(), "a1", "a2"); !errors.Is(err, ErrLastActiveAdmin) {
		t.Fatalf("got %v", err)
	}
}

func TestUserSoftDeleteReturnsNotFoundWhenAlreadyDeleted(t *testing.T) {
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"u1": {ID: "u1", Role: "admin", Status: domainauth.UserStatusActive},
		"u2": {ID: "u2", Role: "user", Status: domainauth.UserStatusDeleted},
	}, activeAdmins: 1}
	flow := newDeleteFlow(users, &outboxFake{}, &revokeFake{}, &revokeFake{}, &revokeFake{})
	if err := flow.Delete(context.Background(), "u1", "u2"); !errors.Is(err, domainauth.ErrUserNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestUserSoftDeleteWorkerRevokesAllCredentialKinds(t *testing.T) {
	outbox := &outboxFake{events: []domainauth.AccessRevocationEvent{{ID: "e1", UserID: "u2", AuthVersion: 2}}}
	token, oauth, mcp := &revokeFake{}, &revokeFake{}, &revokeFake{}
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"u2": {ID: "u2", KratosIdentityID: "identity-2", Status: domainauth.UserStatusDisabled},
	}}
	flow := newDeleteFlow(users, outbox, token, oauth, mcp)
	sessions := &sessionRevokeFake{}
	flow.SetIdentitySessionRevoker(sessions)
	if err := flow.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if token.calls != 1 || oauth.calls != 1 || mcp.calls != 1 || sessions.calls != 1 || sessions.identityID != "identity-2" || !outbox.delivered {
		t.Fatalf("token=%d oauth=%d mcp=%d sessions=%d identity=%q delivered=%v", token.calls, oauth.calls, mcp.calls, sessions.calls, sessions.identityID, outbox.delivered)
	}
}

func TestUserSoftDeleteWorkerRetriesFailures(t *testing.T) {
	outbox := &outboxFake{events: []domainauth.AccessRevocationEvent{{ID: "e1", UserID: "u2", AuthVersion: 2}}}
	flow := newDeleteFlow(&deleteUsersFake{users: map[string]domainauth.User{
		"u2": {ID: "u2", Status: domainauth.UserStatusDisabled},
	}}, outbox, &revokeFake{err: errors.New("db unavailable")}, &revokeFake{}, &revokeFake{})
	if err := flow.ProcessPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !outbox.failed || outbox.delivered {
		t.Fatalf("failed=%v delivered=%v", outbox.failed, outbox.delivered)
	}
}

func TestUserSuspensionCommitsRevocationAndRestoreUsesNewVersion(t *testing.T) {
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"admin": {ID: "admin", Role: "admin", Status: "active", AuthVersion: 1},
		"user":  {ID: "user", Role: "user", Status: "active", AuthVersion: 7},
	}, activeAdmins: 1}
	outbox := &outboxFake{}
	flow := newDeleteFlow(users, outbox, &revokeFake{}, &revokeFake{}, &revokeFake{})

	if err := flow.UpdateStatus(context.Background(), "admin", "user", "disabled"); err != nil {
		t.Fatal(err)
	}
	if got := users.users["user"]; got.Status != "disabled" || got.AuthVersion != 8 {
		t.Fatalf("disabled user=%+v", got)
	}
	if len(outbox.events) != 1 || outbox.events[0].AuthVersion != 8 {
		t.Fatalf("events=%+v", outbox.events)
	}
	if err := flow.UpdateStatus(context.Background(), "admin", "user", "active"); err != nil {
		t.Fatal(err)
	}
	if got := users.users["user"]; got.Status != "active" || got.AuthVersion != 9 {
		t.Fatalf("restored user=%+v", got)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("restore must not create a revocation event: %+v", outbox.events)
	}
}

func TestUserSuspensionProtectsSelfLastAdminAndDeletedTerminal(t *testing.T) {
	users := &deleteUsersFake{users: map[string]domainauth.User{
		"a1": {ID: "a1", Role: "admin", Status: "active", AuthVersion: 1},
		"a2": {ID: "a2", Role: "admin", Status: "active", AuthVersion: 1},
		"d1": {ID: "d1", Role: "user", Status: "deleted", AuthVersion: 2},
	}, activeAdmins: 1}
	flow := newDeleteFlow(users, &outboxFake{}, &revokeFake{}, &revokeFake{}, &revokeFake{})
	if err := flow.UpdateStatus(context.Background(), "a1", "a1", "disabled"); !errors.Is(err, ErrCannotDisableSelf) {
		t.Fatalf("self disable: %v", err)
	}
	if err := flow.UpdateStatus(context.Background(), "a1", "a2", "disabled"); !errors.Is(err, ErrLastActiveAdmin) {
		t.Fatalf("last admin: %v", err)
	}
	if err := flow.UpdateStatus(context.Background(), "a1", "d1", "active"); !errors.Is(err, ErrTerminalUserStatus) {
		t.Fatalf("restore deleted: %v", err)
	}
}
