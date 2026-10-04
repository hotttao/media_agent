// Package token provides current-User checks for legacy Cookie sessions.
// API Token validation belongs to api_token/Talos; encrypted Platform custody
// belongs to platform_token. This query has no old credential repository.
package token

import (
	"context"

	domainauth "media_agent/media_auth/biz/domain/auth"
)

type Service struct {
	userRepo domainauth.UserRepo
}

func NewService(userRepo domainauth.UserRepo) *Service {
	return &Service{userRepo: userRepo}
}

// ValidateSession rejects User disable/delete, version changes and read faults.
func (s *Service) ValidateSession(ctx context.Context, sess domainauth.Session) bool {
	if s == nil || s.userRepo == nil || sess.Sub == "" {
		return false
	}
	u, err := s.userRepo.FindByID(ctx, sess.Sub)
	return err == nil && u.IsActive() && u.AuthVersion == sess.AuthVersion
}
