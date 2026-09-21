package organization

import (
	"context"
	"errors"
	"strings"

	domain "media_agent/media_auth/biz/domain/organization"
)

type Service struct{ repo domain.Repository }

type Summary = domain.Summary
type Membership = domain.Membership

func NewService(repo domain.Repository) *Service { return &Service{repo: repo} }

var (
	ErrNotFound      = domain.ErrNotFound
	ErrNotMember     = domain.ErrNotMember
	ErrForbidden     = errors.New("organization admin only")
	ErrPlatformAdmin = errors.New("platform admin only")
	ErrInvalidName   = errors.New("organization name must not be empty")
	ErrInvalidRole   = errors.New("role must be admin or member")
	ErrInvalidStatus = errors.New("status must be active or disabled")
	ErrUserNotFound  = errors.New("active user not found")
	ErrLastAdmin     = errors.New("organization must retain at least one admin")
)

func (s *Service) List(ctx context.Context, userID string) ([]domain.Summary, error) {
	return s.repo.ListForUser(ctx, userID)
}

func (s *Service) ListActiveIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.repo.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Status == "active" {
			ids = append(ids, row.ID)
		}
	}
	return ids, nil
}
func (s *Service) Create(ctx context.Context, userID, platformRole, name string) (domain.Summary, error) {
	if platformRole != "admin" {
		return domain.Summary{}, ErrPlatformAdmin
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Summary{}, ErrInvalidName
	}
	return s.repo.Create(ctx, name, userID)
}
func (s *Service) Get(ctx context.Context, userID, id string) (domain.Summary, error) {
	m, err := s.repo.GetMembership(ctx, id, userID)
	if err != nil {
		return domain.Summary{}, domain.ErrNotFound
	}
	o, err := s.repo.Get(ctx, id)
	return domain.Summary{Organization: o, MyRole: m.Role}, err
}
func (s *Service) Update(ctx context.Context, userID, id string, name, status *string) (domain.Summary, error) {
	m, err := s.admin(ctx, id, userID)
	if err != nil {
		return domain.Summary{}, err
	}
	if name != nil {
		v := strings.TrimSpace(*name)
		if v == "" {
			return domain.Summary{}, ErrInvalidName
		}
		name = &v
	}
	if status != nil && *status != "active" && *status != "disabled" {
		return domain.Summary{}, ErrInvalidStatus
	}
	o, err := s.repo.Update(ctx, id, name, status)
	return domain.Summary{Organization: o, MyRole: m.Role}, err
}
func (s *Service) ListMembers(ctx context.Context, userID, id string) ([]domain.Membership, error) {
	if _, err := s.repo.GetMembership(ctx, id, userID); err != nil {
		return nil, domain.ErrNotFound
	}
	return s.repo.ListMembers(ctx, id)
}
func (s *Service) PutMember(ctx context.Context, userID, id, target, role string) (domain.Membership, error) {
	if _, err := s.admin(ctx, id, userID); err != nil {
		return domain.Membership{}, err
	}
	if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" {
		return domain.Membership{}, ErrInvalidRole
	}
	exists, err := s.repo.UserExists(ctx, target)
	if err != nil {
		return domain.Membership{}, err
	}
	if !exists {
		return domain.Membership{}, ErrUserNotFound
	}
	if current, err := s.repo.GetMembership(ctx, id, target); err == nil && current.Role == "admin" && role != "admin" {
		n, e := s.repo.CountAdmins(ctx, id)
		if e != nil {
			return domain.Membership{}, e
		}
		if n <= 1 {
			return domain.Membership{}, ErrLastAdmin
		}
	}
	return s.repo.PutMember(ctx, id, target, role)
}
func (s *Service) RemoveMember(ctx context.Context, userID, id, target string) error {
	if _, err := s.admin(ctx, id, userID); err != nil {
		return err
	}
	current, err := s.repo.GetMembership(ctx, id, target)
	if err != nil {
		return err
	}
	if current.Role == "admin" {
		n, e := s.repo.CountAdmins(ctx, id)
		if e != nil {
			return e
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	return s.repo.RemoveMember(ctx, id, target)
}
func (s *Service) GetEntitlements(ctx context.Context, userID, id string) ([]string, error) {
	if _, err := s.repo.GetMembership(ctx, id, userID); err != nil {
		return nil, domain.ErrNotFound
	}
	return s.repo.GetEntitlements(ctx, id)
}
func (s *Service) SetEntitlements(ctx context.Context, userID, platformRole, id string, keys []string) ([]string, error) {
	if platformRole != "admin" {
		return nil, ErrPlatformAdmin
	}
	if _, err := s.admin(ctx, id, userID); err != nil {
		return nil, err
	}
	return s.repo.SetEntitlements(ctx, id, userID, keys)
}
func (s *Service) admin(ctx context.Context, id, userID string) (domain.Membership, error) {
	m, err := s.repo.GetMembership(ctx, id, userID)
	if err != nil {
		return domain.Membership{}, domain.ErrNotFound
	}
	if m.Role != "admin" {
		return domain.Membership{}, ErrForbidden
	}
	return m, nil
}
