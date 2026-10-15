package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"media_agent/media_control/biz/dal"
	"media_agent/media_control/biz/dal/model"
	domain "media_agent/media_control/biz/domain/prompt"
	"time"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db} }

func (r *Repository) ListUnassigned(ctx context.Context, user string) ([]domain.Prompt, error) {
	var rows []model.Prompt
	if err := dal.FromContext(ctx, r.db).Where("owner_user_id = ? AND organization_id IS NULL AND status = ?", user, string(domain.StatusActive)).Order("updated_at DESC,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Prompt, 0, len(rows))
	for i := range rows {
		result = append(result, fromRow(&rows[i]))
	}
	return result, nil
}
func (r *Repository) AssignOrganization(ctx context.Context, p *domain.Prompt, expected int64) error {
	result := dal.FromContext(ctx, r.db).Model(&model.Prompt{}).Where("id = ? AND owner_user_id = ? AND organization_id IS NULL AND version = ? AND status = ?", p.ID, p.OwnerUserID, expected, string(domain.StatusActive)).Updates(map[string]any{"organization_id": p.OrganizationID, "workspace_id": nil, "version": p.Version, "updated_at": p.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return domain.ErrConflict
	}
	return nil
}

type grantRow struct{ PromptID, TeamID, Permission string }

func (r *Repository) Get(ctx context.Context, id string) (*domain.Prompt, error) {
	var row model.Prompt
	err := dal.FromContext(ctx, r.db).Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p := fromRow(&row)
	if err := r.grants(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
func (r *Repository) grants(ctx context.Context, p *domain.Prompt) error {
	var rows []grantRow
	if err := dal.FromContext(ctx, r.db).Table("prompt_grants").Where("prompt_id = ?", p.ID).Order("team_id").Find(&rows).Error; err != nil {
		return err
	}
	p.Grants = make([]domain.Grant, 0, len(rows))
	for _, g := range rows {
		p.Grants = append(p.Grants, domain.Grant{TeamID: g.TeamID, Permission: g.Permission})
	}
	return nil
}
func (r *Repository) List(ctx context.Context, org, user string, teams []string) ([]domain.Prompt, error) {
	db := dal.FromContext(ctx, r.db).Model(&model.Prompt{}).Where("organization_id = ? AND status = ?", org, string(domain.StatusActive))
	if len(teams) == 0 {
		db = db.Where("(owner_user_id = ? OR visibility = 'public')", user)
	} else {
		db = db.Where("(owner_user_id = ? OR visibility = 'public' OR (visibility = 'team' AND EXISTS (SELECT 1 FROM prompt_grants g WHERE g.prompt_id=prompts.id AND g.team_id IN ?)))", user, teams)
	}
	var rows []model.Prompt
	if err := db.Order("updated_at DESC,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Prompt, 0, len(rows))
	for i := range rows {
		p := fromRow(&rows[i])
		if err := r.grants(ctx, &p); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, nil
}

// Save is invoked inside the service transaction; its CAS also serializes ACL edits.
func (r *Repository) Save(ctx context.Context, p *domain.Prompt, expected int64) error {
	db := dal.FromContext(ctx, r.db)
	if expected == 0 {
		if err := db.Table("prompts").Create(map[string]any{"id": p.ID, "organization_id": p.OrganizationID, "owner_user_id": p.OwnerUserID, "name": p.Name, "content": p.Content, "visibility": p.Visibility, "status": string(p.Status), "version": p.Version, "created_at": p.CreatedAt, "updated_at": p.UpdatedAt}).Error; err != nil {
			return err
		}
	} else {
		result := db.Model(&model.Prompt{}).Where("id = ? AND organization_id = ? AND version = ? AND status = ?", p.ID, p.OrganizationID, expected, string(domain.StatusActive)).
			Updates(map[string]any{"name": p.Name, "content": p.Content, "visibility": p.Visibility, "status": string(p.Status), "version": p.Version, "updated_at": p.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return domain.ErrConflict
		}
	}
	if err := db.Table("prompt_grants").Where("prompt_id = ?", p.ID).Delete(&grantRow{}).Error; err != nil {
		return err
	}
	for _, g := range p.Grants {
		if err := db.Table("prompt_grants").Create(map[string]any{"prompt_id": p.ID, "team_id": g.TeamID, "permission": g.Permission}).Error; err != nil {
			return err
		}
	}
	return nil
}

type versionRow struct {
	Version    int64
	Payload    []byte
	RecordedAt *time.Time
}

func fromVersion(row versionRow) (domain.Version, error) {
	var payload struct{ Name, Content string }
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		return domain.Version{}, err
	}
	v := domain.Version{Version: row.Version, Name: payload.Name, Content: payload.Content}
	if row.RecordedAt != nil {
		v.CreatedAt = *row.RecordedAt
	}
	return v, nil
}
func (r *Repository) Versions(ctx context.Context, id string) ([]domain.Version, error) {
	var rows []versionRow
	err := dal.FromContext(ctx, r.db).Table("catalog_resource_versions").Select("version,payload,recorded_at").Where("kind = ? AND resource_id = ?", "media_control.configured_prompt", id).Order("version DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]domain.Version, 0, len(rows))
	for _, row := range rows {
		v, err := fromVersion(row)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}
func (r *Repository) GetVersion(ctx context.Context, id string, version int64) (*domain.Version, error) {
	var row versionRow
	err := dal.FromContext(ctx, r.db).Table("catalog_resource_versions").Select("version,payload,recorded_at").Where("kind = ? AND resource_id = ? AND version = ?", "media_control.configured_prompt", id, version).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v, err := fromVersion(row)
	return &v, err
}
func fromRow(row *model.Prompt) domain.Prompt {
	p := domain.Prompt{ID: row.ID, OwnerUserID: row.OwnerUserID, Name: row.Name, Content: row.Content, Visibility: row.Visibility, Status: domain.Status(row.Status), Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.OrganizationID != nil {
		p.OrganizationID = *row.OrganizationID
	}
	return p
}
