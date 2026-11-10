package workflow

import (
	"context"
	"errors"
	d "media_agent/media_auth/biz/domain/application"
	auth "media_agent/media_auth/biz/domain/auth"
)

type ApplicationAccounts interface {
	AuthenticateLocked(context.Context, string, string) (d.Application, error)
	Issue(context.Context, d.Application, string, string, int64) (string, error)
}
type ManagedAccounts interface {
	Create(context.Context, string, string, string) (auth.User, error)
	Lock(context.Context, string, string) (auth.User, error)
	UpdateProfile(context.Context, auth.User, string, string) (auth.User, error)
}
type ApplicationIdentities interface {
	FindUserIDentity(context.Context, string, string) (string, error)
	CreateApplicationIdentity(context.Context, string, string, string) error
}
type ApplicationUsers struct {
	apps       ApplicationAccounts
	users      ManagedAccounts
	identities ApplicationIdentities
	tx         d.TxRunner
}

func NewApplicationUsers(a ApplicationAccounts, u ManagedAccounts, i ApplicationIdentities, tx d.TxRunner) *ApplicationUsers {
	return &ApplicationUsers{a, u, i, tx}
}

type ApplicationUserCommand struct{ ClientID, Secret, ExternalID, DisplayName, Email string }

func (w *ApplicationUsers) withUser(ctx context.Context, c ApplicationUserCommand, create bool, fn func(context.Context, d.Application, auth.User, bool) error) error {
	if !d.ValidProfile(c.ExternalID, "profile", "") {
		return d.ErrInvalid
	}
	return w.tx.RunInTx(ctx, func(ctx context.Context) error {
		a, e := w.apps.AuthenticateLocked(ctx, c.ClientID, c.Secret)
		if e != nil {
			return e
		}
		id, e := w.identities.FindUserIDentity(ctx, "app:"+a.ID, c.ExternalID)
		if e != nil {
			return e
		}
		var u auth.User
		created := false
		if id == "" {
			if !create {
				return d.ErrNotFound
			}
			u, e = w.users.Create(ctx, a.ID, c.DisplayName, c.Email)
			if e != nil {
				return e
			}
			if e = w.identities.CreateApplicationIdentity(ctx, u.ID, a.ID, c.ExternalID); e != nil {
				return e
			}
			created = true
		} else {
			u, e = w.users.Lock(ctx, id, a.ID)
			if errors.Is(e, auth.ErrManagedUser) {
				return d.ErrForbidden
			}
			if errors.Is(e, auth.ErrUserNotFound) {
				return d.ErrNotFound
			}
			if e != nil {
				return e
			}
		}
		return fn(ctx, a, u, created)
	})
}
func (w *ApplicationUsers) Upsert(ctx context.Context, c ApplicationUserCommand) (d.Profile, bool, error) {
	if !d.ValidProfile(c.ExternalID, c.DisplayName, c.Email) {
		return d.Profile{}, false, d.ErrInvalid
	}
	var p d.Profile
	var created bool
	e := w.withUser(ctx, c, true, func(ctx context.Context, a d.Application, u auth.User, fresh bool) error {
		created = fresh
		if !fresh {
			var err error
			u, err = w.users.UpdateProfile(ctx, u, c.DisplayName, c.Email)
			if err != nil {
				return err
			}
		}
		p = profile(a.ID, c.ExternalID, u)
		return nil
	})
	return p, created, e
}
func (w *ApplicationUsers) Get(ctx context.Context, c ApplicationUserCommand) (d.Profile, error) {
	var p d.Profile
	e := w.withUser(ctx, c, false, func(ctx context.Context, a d.Application, u auth.User, _ bool) error {
		p = profile(a.ID, c.ExternalID, u)
		return nil
	})
	return p, e
}
func (w *ApplicationUsers) Token(ctx context.Context, c ApplicationUserCommand) (string, error) {
	var token string
	e := w.withUser(ctx, c, false, func(ctx context.Context, a d.Application, u auth.User, _ bool) error {
		var err error
		token, err = w.apps.Issue(ctx, a, u.ID, c.ExternalID, u.AuthVersion)
		return err
	})
	return token, e
}
func profile(appID, externalID string, u auth.User) d.Profile {
	return d.Profile{ApplicationID: appID, ExternalID: externalID, UserID: u.ID, DisplayName: u.DisplayName, Email: u.Email}
}

func (w *ApplicationUsers) BusinessToken(ctx context.Context, c ApplicationUserCommand, team string, scopes []string) (string, error) {
	issuer, ok := w.apps.(interface {
		IssueBusiness(context.Context, d.Application, string, string, int64, string, []string) (string, error)
	})
	if !ok {
		return "", d.ErrForbidden
	}
	var token string
	e := w.withUser(ctx, c, false, func(ctx context.Context, a d.Application, u auth.User, _ bool) error {
		var e error
		token, e = issuer.IssueBusiness(ctx, a, u.ID, c.ExternalID, u.AuthVersion, team, scopes)
		return e
	})
	return token, e
}
