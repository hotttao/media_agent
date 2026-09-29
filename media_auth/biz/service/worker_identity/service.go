package workeridentity

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	domain "media_agent/media_auth/biz/domain/worker_identity"
	"media_agent/media_auth/biz/shared/ids"
)

type UserValidator func(context.Context, string) bool
type MembershipValidator func(context.Context, string, string) bool

type Service struct {
	repo       domain.Repository
	userActive UserValidator
	membership MembershipValidator
	now        func() time.Time
}

func NewService(repo domain.Repository) *Service                { return &Service{repo: repo, now: time.Now} }
func (s *Service) SetUserValidator(v UserValidator)             { s.userActive = v }
func (s *Service) SetMembershipValidator(v MembershipValidator) { s.membership = v }

type CreateCommand struct{ OwnerUserID, OrganizationID, Name string }

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (domain.Identity, error) {
	cmd.Name = strings.TrimSpace(cmd.Name)
	if cmd.OwnerUserID == "" || cmd.OrganizationID == "" || cmd.Name == "" || !utf8.ValidString(cmd.Name) || utf8.RuneCountInString(cmd.Name) > 200 {
		return domain.Identity{}, domain.ErrInvalid
	}
	if s.userActive == nil || !s.userActive(ctx, cmd.OwnerUserID) || s.membership == nil || !s.membership(ctx, cmd.OrganizationID, cmd.OwnerUserID) {
		return domain.Identity{}, domain.ErrForbidden
	}
	return s.repo.Create(ctx, domain.Identity{ID: ids.New(), OwnerUserID: cmd.OwnerUserID,
		OrganizationID: cmd.OrganizationID, Name: cmd.Name, Status: domain.StatusActive, CreatedAt: s.now()})
}

func (s *Service) List(ctx context.Context, owner string) ([]domain.Identity, error) {
	if owner == "" {
		return nil, domain.ErrInvalid
	}
	return s.repo.ListByOwner(ctx, owner)
}

func (s *Service) GetOwned(ctx context.Context, owner, id string) (domain.Identity, error) {
	if owner == "" || id == "" {
		return domain.Identity{}, domain.ErrInvalid
	}
	identity, err := s.repo.Get(ctx, id)
	if err != nil {
		return domain.Identity{}, err
	}
	if identity.OwnerUserID != owner {
		return domain.Identity{}, domain.ErrNotFound
	}
	return identity, nil
}

func (s *Service) ValidateActor(ctx context.Context, actor, owner, organization string) bool {
	if !strings.HasPrefix(actor, "Worker:") || owner == "" || organization == "" {
		return false
	}
	id := strings.TrimPrefix(actor, "Worker:")
	if id == "" {
		return false
	}
	identity, err := s.repo.Get(ctx, id)
	return err == nil && identity.ActorID() == actor && identity.Status == domain.StatusActive &&
		identity.OwnerUserID == owner && identity.OrganizationID == organization
}

// ChangeStatus is an internal primitive. Public lifecycle commands must call
// WorkerAccessRevocation so identity and token cleanup intents commit together.
func (s *Service) ChangeStatus(ctx context.Context, owner, id string, next domain.Status) error {
	identity, err := s.GetOwned(ctx, owner, id)
	if err != nil {
		return err
	}
	if !identity.Status.CanTransitionTo(next) {
		return domain.ErrInvalid
	}
	if identity.Status == next {
		return nil
	}
	return s.repo.UpdateStatus(ctx, id, owner, identity.Status, next, s.now())
}
