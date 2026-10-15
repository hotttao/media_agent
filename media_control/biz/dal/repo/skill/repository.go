package skill

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	d "media_agent/media_control/biz/domain/skill"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }

type grantRow struct{ SkillID, TeamID, Permission string }

func fromRow(row model.Skill) d.Skill {
	v := d.Skill{ID: row.ID, OwnerUserID: row.OwnerUserID, Name: row.Name, Description: row.Description, Visibility: row.Visibility, ContentPath: row.ContentPath, Checksum: row.Checksum, DraftChecksum: row.DraftChecksum, SourceKind: row.SourceKind, Status: d.Status(row.Status), Version: row.Version, PublishedVersion: row.PublishedVersion, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Source: d.GitSource{URL: row.GitURL, Branch: row.GitBranch, Subpath: row.GitSubpath, Commit: row.GitCommit, EncryptedToken: row.EncryptedGitToken}}
	if row.OrganizationID != nil {
		v.OrganizationID = *row.OrganizationID
	}
	return v
}
func (r *Repository) Get(ctx context.Context, id string) (*d.Skill, error) {
	var row model.Skill
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v := fromRow(row)
	if err := r.grants(ctx, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
func (r *Repository) grants(ctx context.Context, v *d.Skill) error {
	var rows []grantRow
	if err := dal.FromContext(ctx, r.db).Table("skill_grants").Where("skill_id = ?", v.ID).Order("team_id").Find(&rows).Error; err != nil {
		return err
	}
	v.Grants = make([]d.Grant, 0, len(rows))
	for _, g := range rows {
		v.Grants = append(v.Grants, d.Grant{TeamID: g.TeamID, Permission: g.Permission})
	}
	return nil
}
func (r *Repository) List(ctx context.Context, org, user string, teams []string) ([]d.Skill, error) {
	db := dal.FromContext(ctx, r.db).Model(&model.Skill{}).Where("organization_id = ? AND status <> ?", org, string(d.StatusArchived))
	if len(teams) == 0 {
		db = db.Where("(owner_user_id = ? OR visibility = 'public')", user)
	} else {
		db = db.Where("(owner_user_id = ? OR visibility = 'public' OR (visibility = 'team' AND EXISTS (SELECT 1 FROM skill_grants g WHERE g.skill_id=skills.id AND g.team_id IN ?)))", user, teams)
	}
	var rows []model.Skill
	if err := db.Order("updated_at DESC,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]d.Skill, 0, len(rows))
	for _, row := range rows {
		v := fromRow(row)
		if err := r.grants(ctx, &v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}
func (r *Repository) Save(ctx context.Context, v *d.Skill, expected int64) error {
	db := dal.FromContext(ctx, r.db)
	values := map[string]any{"name": v.Name, "description": v.Description, "visibility": v.Visibility, "content_path": v.ContentPath, "checksum": v.Checksum, "draft_checksum": v.DraftChecksum, "published_version": v.PublishedVersion, "source_kind": v.SourceKind, "git_url": v.Source.URL, "git_branch": v.Source.Branch, "git_subpath": v.Source.Subpath, "git_commit": v.Source.Commit, "encrypted_git_token": v.Source.EncryptedToken, "status": string(v.Status), "version": v.Version, "updated_at": v.UpdatedAt}
	if expected == 0 {
		values["id"] = v.ID
		values["organization_id"] = v.OrganizationID
		values["owner_user_id"] = v.OwnerUserID
		values["created_at"] = v.CreatedAt
		if err := db.Table("skills").Create(values).Error; err != nil {
			return err
		}
	} else {
		result := db.Model(&model.Skill{}).Where("id = ? AND organization_id = ? AND version = ? AND status <> ?", v.ID, v.OrganizationID, expected, string(d.StatusArchived)).Updates(values)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return d.ErrConflict
		}
	}
	if err := db.Table("skill_grants").Where("skill_id = ?", v.ID).Delete(&grantRow{}).Error; err != nil {
		return err
	}
	for _, g := range v.Grants {
		if err := db.Table("skill_grants").Create(map[string]any{"skill_id": v.ID, "team_id": g.TeamID, "permission": g.Permission}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) CreateVersion(ctx context.Context, v *d.Version) error {
	return dal.FromContext(ctx, r.db).Table("skill_versions").Create(map[string]any{"skill_id": v.SkillID, "version": v.Version, "checksum": v.Checksum, "slug": v.Slug, "description": v.Description, "published_by": v.PublishedBy, "created_at": v.CreatedAt}).Error
}
func (r *Repository) Versions(ctx context.Context, id string) ([]d.Version, error) {
	result := []d.Version{}
	err := dal.FromContext(ctx, r.db).Table("skill_versions").Where("skill_id = ?", id).Order("version DESC").Find(&result).Error
	return result, err
}
func (r *Repository) GetVersion(ctx context.Context, id string, version int64) (*d.Version, error) {
	var v d.Version
	err := dal.FromContext(ctx, r.db).Table("skill_versions").Where("skill_id = ? AND version = ?", id, version).Take(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, d.ErrNotFound
	}
	return &v, err
}
func (r *Repository) ListUnassigned(ctx context.Context, user string) ([]d.Skill, error) {
	var rows []model.Skill
	if err := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND organization_id IS NULL AND status <> ?", user, string(d.StatusArchived)).Order("updated_at DESC,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]d.Skill, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRow(row))
	}
	return result, nil
}
func (r *Repository) AssignOrganization(ctx context.Context, v *d.Skill, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Skill{}).Where("id = ? AND owner_user_id = ? AND organization_id IS NULL AND version = ? AND status <> ?", v.ID, v.OwnerUserID, expected, string(d.StatusArchived)).Updates(map[string]any{"organization_id": v.OrganizationID, "workspace_id": nil, "version": v.Version, "updated_at": v.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return d.ErrConflict
	}
	return nil
}
