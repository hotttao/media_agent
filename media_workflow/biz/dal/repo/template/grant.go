package template

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	"media_agent/media_workflow/biz/dal/query"
	domain "media_agent/media_workflow/biz/domain/template"
)

// GrantRepo 实现 domain.GrantRepo。
type GrantRepo struct{ db *gorm.DB }

func NewGrantRepo(db *gorm.DB) *GrantRepo { return &GrantRepo{db: db} }

func (r *GrantRepo) AddGrant(ctx context.Context, g *domain.Grant) error {
	row := &model.TemplateGrant{
		TemplateID: g.TemplateID, TeamID: g.TeamID,
		Permission: string(g.Permission), GrantedBy: g.GrantedBy, GrantedAt: time.Now(),
	}
	return query.Use(dal.FromContext(ctx, r.db)).TemplateGrant.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "template_id"}, {Name: "team_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"permission", "granted_by", "granted_at"}),
		}).Create(row)
}

func (r *GrantRepo) RemoveGrant(ctx context.Context, templateID, teamID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.TemplateGrant.WithContext(ctx).
		Where(Q.TemplateGrant.TemplateID.Eq(templateID), Q.TemplateGrant.TeamID.Eq(teamID)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GrantRepo) ListGrants(ctx context.Context, templateID string) ([]*domain.Grant, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.TemplateGrant.WithContext(ctx).Where(Q.TemplateGrant.TemplateID.Eq(templateID)).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Grant, len(rows))
	for i := range rows {
		out[i] = &domain.Grant{
			TemplateID: rows[i].TemplateID, TeamID: rows[i].TeamID,
			Permission: domain.Permission(rows[i].Permission), GrantedBy: rows[i].GrantedBy,
			GrantedAt: rows[i].GrantedAt,
		}
	}
	return out, nil
}
