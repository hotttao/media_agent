package token

import (
	"context"
	"errors"
	"testing"

	domainauth "media_agent/media_auth/biz/domain/auth"
)

type userRepoStub struct {
	domainauth.UserRepo
	user domainauth.User
	err  error
}

func (r *userRepoStub) FindByID(context.Context, string) (domainauth.User, error) {
	return r.user, r.err
}

func TestValidateSessionUsesCurrentUserAndVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  string
		version int64
		err     error
		want    bool
	}{
		{"active", "active", 3, nil, true},
		{"disabled", "disabled", 3, nil, false},
		{"deleted", "deleted", 3, nil, false},
		{"revoked-version", "active", 4, nil, false},
		{"repository-failure", "active", 3, errors.New("unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &userRepoStub{user: domainauth.User{ID: "owner", Status: tc.status, AuthVersion: tc.version}, err: tc.err}
			if got := NewService(repo).ValidateSession(t.Context(), domainauth.Session{Sub: "owner", AuthVersion: 3}); got != tc.want {
				t.Fatalf("session activity=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestValidateSessionRejectsMissingPrerequisites(t *testing.T) {
	var absent *Service
	if absent.ValidateSession(t.Context(), domainauth.Session{}) || NewService(nil).ValidateSession(t.Context(), domainauth.Session{Sub: "owner"}) || NewService(&userRepoStub{}).ValidateSession(t.Context(), domainauth.Session{}) {
		t.Fatal("missing session prerequisites accepted")
	}
}
