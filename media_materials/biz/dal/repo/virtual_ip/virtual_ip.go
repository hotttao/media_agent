package virtual_ip

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_materials/biz/dal"
	"media_agent/media_materials/biz/dal/model"
	"media_agent/media_materials/biz/dal/query"
	domain "media_agent/media_materials/biz/domain/virtual_ip"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Save(ctx context.Context, v *domain.VirtualIp) error {
	row := toModel(v)
	row.CreatedAt = time.Now()
	row.UpdatedAt = time.Now()
	return query.Use(dal.FromContext(ctx, r.db)).VirtualIp.WithContext(ctx).Create(row)
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.VirtualIp, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.VirtualIp.WithContext(ctx).
		Where(Q.VirtualIp.ID.Eq(id), Q.VirtualIp.DeletedAt.IsNull()).First()
	if err != nil {
		return nil, err
	}
	return toDomain(row), nil
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.VirtualIp, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.VirtualIp.WithContext(ctx).Where(Q.VirtualIp.DeletedAt.IsNull())
	if q.Scope != "" {
		tx = tx.Where(Q.VirtualIp.Scope.Eq(q.Scope))
	}
	if q.Name != "" {
		tx = tx.Where(Q.VirtualIp.Name.Like("%" + q.Name + "%"))
	}
	if q.Status != "" {
		tx = tx.Where(Q.VirtualIp.Status.Eq(q.Status))
	}
	if q.Visibility != "" {
		tx = tx.Where(Q.VirtualIp.Visibility.Eq(q.Visibility))
	}
	total, err := tx.Count()
	if err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	rows, err := tx.Order(Q.VirtualIp.UpdatedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.VirtualIp, len(rows))
	for i := range rows {
		out[i] = toDomain(rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, v *domain.VirtualIp) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.VirtualIp.WithContext(ctx).
		Where(Q.VirtualIp.ID.Eq(v.ID), Q.VirtualIp.DeletedAt.IsNull()).
		Updates(map[string]any{
			"name":           v.Name,
			"description":    v.Description,
			"avatar_url":     v.AvatarURL,
			"full_body_url":  v.FullBodyURL,
			"three_view_url": v.ThreeViewURL,
			"nine_view_url":  v.NineViewURL,
			"status":         string(v.Status),
			"visibility":     string(v.Visibility),
			"updated_at":     time.Now(),
		})
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.VirtualIp.WithContext(ctx).
		Where(Q.VirtualIp.ID.Eq(id), Q.VirtualIp.DeletedAt.IsNull()).
		Updates(map[string]any{"deleted_at": time.Now()})
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func toModel(v *domain.VirtualIp) *model.VirtualIp {
	return &model.VirtualIp{
		ID: v.ID, Scope: v.Scope, Name: v.Name, Description: v.Description,
		AvatarURL: v.AvatarURL, FullBodyURL: v.FullBodyURL,
		ThreeViewURL: v.ThreeViewURL, NineViewURL: v.NineViewURL,
		Status: string(v.Status), Visibility: string(v.Visibility), OwnerUserID: v.OwnerUserID,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomain(row *model.VirtualIp) *domain.VirtualIp {
	return &domain.VirtualIp{
		ID: row.ID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		AvatarURL: row.AvatarURL, FullBodyURL: row.FullBodyURL,
		ThreeViewURL: row.ThreeViewURL, NineViewURL: row.NineViewURL,
		Status: domain.Status(row.Status), Visibility: domain.Visibility(row.Visibility),
		OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// —— GrantRepo ——

type GrantRepo struct{ db *gorm.DB }

func NewGrantRepo(db *gorm.DB) *GrantRepo { return &GrantRepo{db: db} }

func (r *GrantRepo) AddGrant(ctx context.Context, g *domain.Grant) error {
	row := &model.VirtualIpGrant{
		VirtualIPID: g.VirtualIPID, TeamID: g.TeamID,
		Permission: string(g.Permission), GrantedBy: g.GrantedBy, GrantedAt: time.Now(),
	}
	// ON CONFLICT (virtual_ip_id, team_id) DO UPDATE — 覆盖 permission
	return query.Use(dal.FromContext(ctx, r.db)).VirtualIpGrant.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "virtual_ip_id"}, {Name: "team_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"permission", "granted_by", "granted_at"}),
		}).Create(row)
}

func (r *GrantRepo) RemoveGrant(ctx context.Context, virtualIPID, teamID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	res, err := Q.VirtualIpGrant.WithContext(ctx).
		Where(Q.VirtualIpGrant.VirtualIPID.Eq(virtualIPID), Q.VirtualIpGrant.TeamID.Eq(teamID)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GrantRepo) ListGrants(ctx context.Context, virtualIPID string) ([]*domain.Grant, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.VirtualIpGrant.WithContext(ctx).Where(Q.VirtualIpGrant.VirtualIPID.Eq(virtualIPID)).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Grant, len(rows))
	for i := range rows {
		out[i] = &domain.Grant{
			VirtualIPID: rows[i].VirtualIPID, TeamID: rows[i].TeamID,
			Permission: domain.Permission(rows[i].Permission), GrantedBy: rows[i].GrantedBy,
			GrantedAt: rows[i].GrantedAt,
		}
	}
	return out, nil
}

func (r *GrantRepo) ListByTeam(ctx context.Context, teamID string) ([]*domain.Grant, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.VirtualIpGrant.WithContext(ctx).Where(Q.VirtualIpGrant.TeamID.Eq(teamID)).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Grant, len(rows))
	for i := range rows {
		out[i] = &domain.Grant{
			VirtualIPID: rows[i].VirtualIPID, TeamID: rows[i].TeamID,
			Permission: domain.Permission(rows[i].Permission), GrantedBy: rows[i].GrantedBy,
			GrantedAt: rows[i].GrantedAt,
		}
	}
	return out, nil
}
