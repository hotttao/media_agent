package skill

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"media_agent/hertz_infra/authn/resourceaccess"
	"media_agent/media_control/biz/dal"
	d "media_agent/media_control/biz/domain/skill"
	"strings"
	"time"
	"unicode/utf8"
)

type Access interface {
	Resolve(context.Context, string, string) (resourceaccess.Facts, error)
	Team(context.Context, string, string) (resourceaccess.Team, error)
}
type View struct {
	d.Skill
	MyPermission string
}
type Service struct {
	repo   d.Repository
	tx     dal.TxRunner
	access Access
}

func New(repo d.Repository, tx dal.TxRunner, access Access) *Service {
	return &Service{repo, tx, access}
}

type CreateCommand struct {
	OwnerUserID, OrganizationID, Name, Description, Visibility, DraftChecksum, SourceKind string
	Grants                                                                                []d.Grant
	Source                                                                                d.GitSource
}
type UpdateCommand struct {
	SkillID, OwnerUserID, Name, Description string
	ExpectedVersion                         int64
}
type StatusCommand struct {
	SkillID, OwnerUserID, Status string
	ExpectedVersion              int64
}
type ArchiveCommand struct {
	SkillID, OwnerUserID string
	ExpectedVersion      int64
}
type ACLCommand struct {
	SkillID, UserID, Visibility string
	ExpectedVersion             int64
	Grants                      []d.Grant
}

func (s *Service) facts(ctx context.Context, user, org string) (resourceaccess.Facts, error) {
	if user == "" || org == "" {
		return resourceaccess.Facts{}, d.ErrNotFound
	}
	if s.access == nil {
		return resourceaccess.Facts{}, d.ErrUnavailable
	}
	facts, err := s.access.Resolve(ctx, user, org)
	if err != nil {
		return facts, fmt.Errorf("%w: %v", d.ErrUnavailable, err)
	}
	if !facts.Member {
		return facts, d.ErrNotFound
	}
	return facts, nil
}
func (s *Service) AuthorizeOrganization(ctx context.Context, user, org string) error {
	_, err := s.facts(ctx, user, org)
	return err
}
func (s *Service) validateACL(ctx context.Context, user, org, visibility string, grants []d.Grant) error {
	if err := d.ValidateACL(visibility, grants); err != nil {
		return err
	}
	for _, g := range grants {
		team, err := s.access.Team(ctx, user, g.TeamID)
		if err != nil {
			return fmt.Errorf("%w: %v", d.ErrUnavailable, err)
		}
		if team.OrganizationID != org || !team.TeamActive || !team.OrganizationActive {
			return d.ErrForbidden
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, c CreateCommand) (*View, error) {
	if c.OrganizationID == "" || !d.IsChecksum(c.DraftChecksum) {
		return nil, d.ErrInvalid
	}
	if _, err := s.facts(ctx, c.OwnerUserID, c.OrganizationID); err != nil {
		return nil, err
	}
	name, description, err := d.Validate(c.Name, c.Description)
	if err != nil {
		return nil, err
	}
	if c.Visibility == "" {
		c.Visibility = "private"
	}
	if err := s.validateACL(ctx, c.OwnerUserID, c.OrganizationID, c.Visibility, c.Grants); err != nil {
		return nil, err
	}
	if c.SourceKind != "native" && c.SourceKind != "upload" && c.SourceKind != "git" {
		return nil, d.ErrInvalid
	}
	if c.SourceKind == "git" {
		if err := d.ValidateGit(c.Source); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	v := d.Skill{ID: uuid.NewString(), OrganizationID: c.OrganizationID, OwnerUserID: c.OwnerUserID, Name: name, Description: description, Visibility: c.Visibility, DraftChecksum: c.DraftChecksum, SourceKind: c.SourceKind, Source: c.Source, Grants: append([]d.Grant{}, c.Grants...), Status: d.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.tx.RunInTx(ctx, func(ctx context.Context) error { return s.repo.Save(ctx, &v, 0) }); err != nil {
		return nil, err
	}
	return &View{v, "owner"}, nil
}
func (s *Service) Get(ctx context.Context, id, user string) (*View, error) {
	if id == "" || user == "" {
		return nil, d.ErrNotFound
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OrganizationID == "" {
		if user != v.OwnerUserID {
			return nil, d.ErrNotFound
		}
		return &View{*v, "owner"}, nil
	}
	facts, err := s.facts(ctx, user, v.OrganizationID)
	if err != nil {
		return nil, err
	}
	permission := v.Permission(user, facts.TeamIDs)
	if permission == "none" {
		return nil, d.ErrNotFound
	}
	return &View{*v, permission}, nil
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
	result := make([]View, 0, len(rows))
	for _, v := range rows {
		permission := v.Permission(user, facts.TeamIDs)
		if permission != "none" && v.OrganizationID == org {
			result = append(result, View{v, permission})
		}
	}
	return result, nil
}
func (s *Service) Writable(ctx context.Context, id, user string, expected int64, ownerOnly bool) (*View, error) {
	if expected <= 0 {
		return nil, d.ErrInvalid
	}
	v, err := s.Get(ctx, id, user)
	if err != nil {
		return nil, err
	}
	if v.MyPermission != "owner" && (ownerOnly || v.MyPermission != "editor") {
		return nil, d.ErrForbidden
	}
	if v.OrganizationID == "" {
		return nil, d.ErrInvalid
	}
	if v.Status == d.StatusArchived {
		return nil, d.ErrArchived
	}
	if v.Version != expected {
		return nil, d.ErrConflict
	}
	return v, nil
}
func (s *Service) save(ctx context.Context, v *View, expected int64) (*View, error) {
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error { return s.repo.Save(ctx, &v.Skill, expected) })
	return v, err
}
func (s *Service) Update(ctx context.Context, c UpdateCommand) (*View, error) {
	name, description, err := d.Validate(c.Name, c.Description)
	if err != nil {
		return nil, err
	}
	v, err := s.Writable(ctx, c.SkillID, c.OwnerUserID, c.ExpectedVersion, false)
	if err != nil {
		return nil, err
	}
	v.Name, v.Description = name, description
	return s.save(ctx, v, c.ExpectedVersion)
}
func (s *Service) SetStatus(ctx context.Context, c StatusCommand) (*View, error) {
	status, err := d.ParseStatus(c.Status)
	if err != nil {
		return nil, err
	}
	v, err := s.Writable(ctx, c.SkillID, c.OwnerUserID, c.ExpectedVersion, true)
	if err != nil {
		return nil, err
	}
	if v.Status == status {
		return v, nil
	}
	v.Status = status
	return s.save(ctx, v, c.ExpectedVersion)
}
func (s *Service) Archive(ctx context.Context, c ArchiveCommand) (*View, error) {
	v, err := s.Writable(ctx, c.SkillID, c.OwnerUserID, c.ExpectedVersion, true)
	if err != nil {
		return nil, err
	}
	v.Status = d.StatusArchived
	return s.save(ctx, v, c.ExpectedVersion)
}
func (s *Service) SetACL(ctx context.Context, c ACLCommand) (*View, error) {
	v, err := s.Writable(ctx, c.SkillID, c.UserID, c.ExpectedVersion, true)
	if err != nil {
		return nil, err
	}
	if err := s.validateACL(ctx, c.UserID, v.OrganizationID, c.Visibility, c.Grants); err != nil {
		return nil, err
	}
	v.Visibility = c.Visibility
	v.Grants = append([]d.Grant{}, c.Grants...)
	return s.save(ctx, v, c.ExpectedVersion)
}
func (s *Service) SaveDraft(ctx context.Context, id, user string, expected int64, checksum string) (*View, error) {
	if !d.IsChecksum(checksum) {
		return nil, d.ErrInvalid
	}
	v, err := s.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	v.DraftChecksum = checksum
	return s.save(ctx, v, expected)
}
func (s *Service) SaveSource(ctx context.Context, id, user string, expected int64, source d.GitSource) (*View, error) {
	if err := d.ValidateGit(source); err != nil {
		return nil, err
	}
	v, err := s.Writable(ctx, id, user, expected, true)
	if err != nil {
		return nil, err
	}
	v.Source = source
	v.SourceKind = "git"
	return s.save(ctx, v, expected)
}
func (s *Service) SaveSyncedDraft(ctx context.Context, id, user string, expected int64, checksum, commit string) (*View, error) {
	if !d.IsChecksum(checksum) {
		return nil, d.ErrInvalid
	}
	v, err := s.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	if v.SourceKind != "git" {
		return nil, d.ErrInvalid
	}
	v.DraftChecksum = checksum
	v.Source.Commit = commit
	return s.save(ctx, v, expected)
}
func (s *Service) Publish(ctx context.Context, id, user string, expected int64, draftChecksum, checksum, slug, description string) (*View, error) {
	if !d.IsChecksum(checksum) || !d.IsSlug(slug) || strings.TrimSpace(description) == "" || utf8.RuneCountInString(description) > 1024 {
		return nil, d.ErrInvalid
	}
	v, err := s.Writable(ctx, id, user, expected, false)
	if err != nil {
		return nil, err
	}
	if draftChecksum != v.DraftChecksum {
		return nil, d.ErrConflict
	}
	if v.PublishedVersion > 0 && v.Checksum == checksum {
		if v.DraftChecksum != checksum {
			v.DraftChecksum = checksum
			return s.save(ctx, v, expected)
		}
		return v, nil
	}
	v.PublishedVersion++
	v.Checksum = checksum
	v.DraftChecksum = checksum
	v.ContentPath = "sha256/" + checksum
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	history := &d.Version{SkillID: id, Version: v.PublishedVersion, Checksum: checksum, Slug: slug, Description: description, PublishedBy: user, CreatedAt: v.UpdatedAt}
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Save(ctx, &v.Skill, expected); err != nil {
			return err
		}
		return s.repo.CreateVersion(ctx, history)
	})
	return v, err
}
func (s *Service) Versions(ctx context.Context, id, user string) ([]d.Version, error) {
	if _, err := s.Get(ctx, id, user); err != nil {
		return nil, err
	}
	return s.repo.Versions(ctx, id)
}
func (s *Service) Version(ctx context.Context, id, user string, version int64) (*d.Version, error) {
	if version <= 0 {
		return nil, d.ErrInvalid
	}
	if _, err := s.Get(ctx, id, user); err != nil {
		return nil, err
	}
	return s.repo.GetVersion(ctx, id, version)
}
func (s *Service) SelectedVersion(ctx context.Context, id, user, org string, version int64) (*d.Version, error) {
	return s.readVersion(ctx, id, user, org, version, true)
}
func (s *Service) ReadableVersion(ctx context.Context, id, user, org string, version int64) (*d.Version, error) {
	return s.readVersion(ctx, id, user, org, version, false)
}
func (s *Service) readVersion(ctx context.Context, id, user, org string, version int64, active bool) (*d.Version, error) {
	v, err := s.Get(ctx, id, user)
	if err != nil {
		return nil, err
	}
	if v.OrganizationID == "" || v.OrganizationID != org {
		return nil, d.ErrForbidden
	}
	if active && v.Status != d.StatusActive {
		return nil, d.ErrInvalid
	}
	if version <= 0 {
		return nil, d.ErrInvalid
	}
	return s.repo.GetVersion(ctx, id, version)
}
func (s *Service) Unassigned(ctx context.Context, user string) ([]View, error) {
	if user == "" {
		return nil, d.ErrNotFound
	}
	rows, err := s.repo.ListUnassigned(ctx, user)
	if err != nil {
		return nil, err
	}
	result := make([]View, 0, len(rows))
	for _, v := range rows {
		result = append(result, View{v, "owner"})
	}
	return result, nil
}
func (s *Service) AssignOrganization(ctx context.Context, id, user, org string, expected int64) (*View, error) {
	if expected <= 0 || org == "" {
		return nil, d.ErrInvalid
	}
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != user {
		return nil, d.ErrNotFound
	}
	if v.OrganizationID != "" || v.Version != expected {
		return nil, d.ErrConflict
	}
	if v.Status == d.StatusArchived {
		return nil, d.ErrArchived
	}
	if _, err := s.facts(ctx, user, org); err != nil {
		return nil, err
	}
	v.OrganizationID = org
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error { return s.repo.AssignOrganization(ctx, v, expected) })
	return &View{*v, "owner"}, err
}
