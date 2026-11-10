package auth

import (
	"context"
	d "media_agent/media_auth/biz/domain/auth"
)

type ManagedUsers struct{ repo d.ManagedUserRepo }

func NewManagedUsers(repo d.ManagedUserRepo) *ManagedUsers { return &ManagedUsers{repo} }
func (s *ManagedUsers) Create(ctx context.Context, applicationID, name, email string) (d.User, error) {
	return s.repo.CreateManaged(ctx, applicationID, name, email)
}
func (s *ManagedUsers) Lock(ctx context.Context, userID, applicationID string) (d.User, error) {
	u, e := s.repo.LockManaged(ctx, userID)
	if e != nil {
		return d.User{}, e
	}
	if u.ManagedBy != applicationID || !u.IsActive() || u.Role != "user" {
		return d.User{}, d.ErrManagedUser
	}
	return u, nil
}
func (s *ManagedUsers) UpdateProfile(ctx context.Context, u d.User, name, email string) (d.User, error) {
	if u.ManagedBy == "" || !u.IsActive() {
		return d.User{}, d.ErrManagedUser
	}
	if e := s.repo.UpdateManagedProfile(ctx, u.ID, name, email); e != nil {
		return d.User{}, e
	}
	u.DisplayName = name
	u.Email = email
	return u, nil
}
