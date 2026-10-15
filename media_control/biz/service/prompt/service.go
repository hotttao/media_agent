package prompt

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/media_control/biz/dal"
	domain "media_agent/media_control/biz/domain/prompt"
	"strings"
	"time"
)

type Access interface {
	Resolve(context.Context, string, string) (resourceaccess.Facts, error)
	Team(context.Context, string, string) (resourceaccess.Team, error)
}
type CreateCommand struct {
	OwnerUserID, OrganizationID, Name, Content, Visibility string
	Grants                                                 []domain.Grant
}
type UpdateCommand struct {
	PromptID, OwnerUserID, Name, Content string
	ExpectedVersion                      int64
}
type ArchiveCommand struct {
	PromptID, OwnerUserID string
	ExpectedVersion       int64
}
type ACLCommand struct {
	PromptID, UserID, Visibility string
	ExpectedVersion              int64
	Grants                       []domain.Grant
}
type RollbackCommand struct {
	PromptID, UserID               string
	ExpectedVersion, TargetVersion int64
}
type View struct {
	domain.Prompt
	MyPermission string
}
type Service struct {
	repo   domain.Repository
	tx     dal.TxRunner
	access Access
}

func New(repo domain.Repository, tx dal.TxRunner, access Access) *Service {
	return &Service{repo, tx, access}
}
func (s *Service) facts(ctx context.Context, user, org string) (resourceaccess.Facts, error) {
	if user == "" || org == "" {
		return resourceaccess.Facts{}, domain.ErrNotFound
	}
	if s.access == nil {
		return resourceaccess.Facts{}, domain.ErrUnavailable
	}
	facts, err := s.access.Resolve(ctx, user, org)
	if err != nil {
		return facts, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
	}
	if !facts.Member {
		return facts, domain.ErrNotFound
	}
	return facts, nil
}
func (s *Service) validateGrants(ctx context.Context, user, org, visibility string, grants []domain.Grant) error {
	if err := domain.ValidateACL(visibility, grants); err != nil {
		return err
	}
	for _, g := range grants {
		team, err := s.access.Team(ctx, user, g.TeamID)
		if err != nil {
			return fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
		}
		if team.OrganizationID != org || !team.TeamActive || !team.OrganizationActive {
			return domain.ErrForbidden
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*View, error) {
	cmd.OrganizationID = strings.TrimSpace(cmd.OrganizationID)
	if cmd.OrganizationID == "" {
		return nil, domain.ErrInvalid
	}
	if _, err := s.facts(ctx, cmd.OwnerUserID, cmd.OrganizationID); err != nil {
		return nil, err
	}
	name, content, err := domain.Validate(cmd.Name, cmd.Content)
	if err != nil {
		return nil, err
	}
	if cmd.Visibility == "" {
		cmd.Visibility = "private"
	}
	if err := s.validateGrants(ctx, cmd.OwnerUserID, cmd.OrganizationID, cmd.Visibility, cmd.Grants); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := domain.Prompt{ID: uuid.NewString(), OrganizationID: cmd.OrganizationID, OwnerUserID: cmd.OwnerUserID, Name: name, Content: content, Visibility: cmd.Visibility, Grants: append([]domain.Grant{}, cmd.Grants...), Status: domain.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(c context.Context) error { return s.repo.Save(c, &p, 0) }); err != nil {
		return nil, err
	}
	return &View{p, "owner"}, nil
}
func (s *Service) List(ctx context.Context, user, org string) ([]View, error) {
	facts, err := s.facts(ctx, user, org)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.List(ctx, org, user, facts.TeamIDs)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, p := range rows {
		permission := p.Permission(user, facts.TeamIDs)
		if permission != "none" && p.OrganizationID == org {
			views = append(views, View{p, permission})
		}
	}
	return views, nil
}
func (s *Service) Get(ctx context.Context, id, user string) (*View, error) {
	if id == "" || user == "" {
		return nil, domain.ErrNotFound
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.OrganizationID == "" {
		if p.OwnerUserID != user {
			return nil, domain.ErrNotFound
		}
		return &View{*p, "owner"}, nil
	}
	facts, err := s.facts(ctx, user, p.OrganizationID)
	if err != nil {
		return nil, err
	}
	permission := p.Permission(user, facts.TeamIDs)
	if permission == "none" {
		return nil, domain.ErrNotFound
	}
	return &View{*p, permission}, nil
}
func (s *Service) editable(ctx context.Context, id, user string, expected int64, ownerOnly bool) (*View, error) {
	if expected <= 0 {
		return nil, domain.ErrInvalid
	}
	v, err := s.Get(ctx, id, user)
	if err != nil {
		return nil, err
	}
	if v.MyPermission != "owner" && (ownerOnly || v.MyPermission != "editor") {
		return nil, domain.ErrForbidden
	}
	if v.OrganizationID == "" {
		return nil, domain.ErrInvalid
	}
	if v.Status != domain.StatusActive {
		return nil, domain.ErrArchived
	}
	if v.Version != expected {
		return nil, domain.ErrConflict
	}
	return v, nil
}
func (s *Service) save(ctx context.Context, v *View, expected int64) (*View, error) {
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	err := s.tx.RunInTx(ctx, func(c context.Context) error { return s.repo.Save(c, &v.Prompt, expected) })
	return v, err
}
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*View, error) {
	name, content, err := domain.Validate(cmd.Name, cmd.Content)
	if err != nil {
		return nil, err
	}
	v, err := s.editable(ctx, cmd.PromptID, cmd.OwnerUserID, cmd.ExpectedVersion, false)
	if err != nil {
		return nil, err
	}
	if v.Name == name && v.Content == content {
		return v, nil
	}
	v.Name, v.Content = name, content
	return s.save(ctx, v, cmd.ExpectedVersion)
}
func (s *Service) Archive(ctx context.Context, cmd ArchiveCommand) (*View, error) {
	v, err := s.editable(ctx, cmd.PromptID, cmd.OwnerUserID, cmd.ExpectedVersion, true)
	if err != nil {
		return nil, err
	}
	v.Status = domain.StatusArchived
	return s.save(ctx, v, cmd.ExpectedVersion)
}
func (s *Service) SetACL(ctx context.Context, cmd ACLCommand) (*View, error) {
	v, err := s.editable(ctx, cmd.PromptID, cmd.UserID, cmd.ExpectedVersion, true)
	if err != nil {
		return nil, err
	}
	if err := s.validateGrants(ctx, cmd.UserID, v.OrganizationID, cmd.Visibility, cmd.Grants); err != nil {
		return nil, err
	}
	v.Visibility = cmd.Visibility
	v.Grants = append([]domain.Grant{}, cmd.Grants...)
	return s.save(ctx, v, cmd.ExpectedVersion)
}
func (s *Service) Versions(ctx context.Context, id, user string) ([]domain.Version, error) {
	if _, err := s.Get(ctx, id, user); err != nil {
		return nil, err
	}
	return s.repo.Versions(ctx, id)
}
func (s *Service) Version(ctx context.Context, id, user string, version int64) (*domain.Version, error) {
	if version <= 0 {
		return nil, domain.ErrInvalid
	}
	if _, err := s.Get(ctx, id, user); err != nil {
		return nil, err
	}
	return s.repo.GetVersion(ctx, id, version)
}
func (s *Service) Rollback(ctx context.Context, cmd RollbackCommand) (*View, error) {
	v, err := s.editable(ctx, cmd.PromptID, cmd.UserID, cmd.ExpectedVersion, false)
	if err != nil {
		return nil, err
	}
	if cmd.TargetVersion <= 0 {
		return nil, domain.ErrInvalid
	}
	history, err := s.repo.GetVersion(ctx, cmd.PromptID, cmd.TargetVersion)
	if err != nil {
		return nil, err
	}
	v.Content = history.Content
	return s.save(ctx, v, cmd.ExpectedVersion)
}

// SelectedVersion checks today's ACL and tenant against an immutable content version.
func (s *Service) SelectedVersion(ctx context.Context, id, user, organization string, version int64) (*domain.Version, error) {
	return s.readVersion(ctx, id, user, organization, version, true)
}
func (s *Service) ReadableVersion(ctx context.Context, id, user, organization string, version int64) (*domain.Version, error) {
	return s.readVersion(ctx, id, user, organization, version, false)
}
func (s *Service) readVersion(ctx context.Context, id, user, organization string, version int64, active bool) (*domain.Version, error) {
	if version <= 0 {
		return nil, domain.ErrInvalid
	}
	v, err := s.Get(ctx, id, user)
	if err != nil {
		return nil, err
	}
	if v.OrganizationID == "" || v.OrganizationID != organization {
		return nil, domain.ErrForbidden
	}
	if active && v.Status != domain.StatusActive {
		return nil, domain.ErrArchived
	}
	return s.repo.GetVersion(ctx, id, version)
}

func (s *Service) Unassigned(ctx context.Context, user string) ([]View, error) {
	if user == "" {
		return nil, domain.ErrNotFound
	}
	rows, err := s.repo.ListUnassigned(ctx, user)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(rows))
	for _, v := range rows {
		views = append(views, View{v, "owner"})
	}
	return views, nil
}
func (s *Service) AssignOrganization(ctx context.Context, id, user, organization string, expected int64) (*View, error) {
	if expected <= 0 || strings.TrimSpace(organization) == "" {
		return nil, domain.ErrInvalid
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != user {
		return nil, domain.ErrNotFound
	}
	if v.OrganizationID != "" || v.Version != expected {
		return nil, domain.ErrConflict
	}
	if v.Status != domain.StatusActive {
		return nil, domain.ErrArchived
	}
	if _, err := s.facts(ctx, user, organization); err != nil {
		return nil, err
	}
	v.OrganizationID = organization
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	if err := s.tx.RunInTx(ctx, func(c context.Context) error { return s.repo.AssignOrganization(c, v, expected) }); err != nil {
		return nil, err
	}
	return &View{*v, "owner"}, nil
}
