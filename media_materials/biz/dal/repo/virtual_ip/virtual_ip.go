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
	return dal.FromContext(ctx, r.db).WithContext(ctx).Create(row).Error
}

func (r *Repo) Get(ctx context.Context, id string) (*domain.VirtualIp, error) {
	var row model.VirtualIp
	err := dal.FromContext(ctx, r.db).WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&row).Error
	return toDomain(&row), err
}

func (r *Repo) List(ctx context.Context, q domain.ListQuery) ([]*domain.VirtualIp, int, error) {
	tx := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.VirtualIp{}).Where("deleted_at IS NULL")
	if q.OrganizationID != "" {
		tx = tx.Where("organization_id = ?", q.OrganizationID)
	}
	if q.Scope != "" {
		tx = tx.Where("scope = ?", q.Scope)
	}
	if q.Name != "" {
		tx = tx.Where("name ILIKE ?", "%"+q.Name+"%")
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Visibility != "" {
		tx = tx.Where("visibility = ?", q.Visibility)
	}
	if len(q.CallerTeamIDs) > 0 {
		tx = tx.Where(`owner_user_id = ? OR visibility = 'public' OR (
			visibility = 'team' AND EXISTS (
				SELECT 1 FROM media_materials.virtual_ip_grants grants
				WHERE grants.virtual_ip_id = media_materials.virtual_ips.id
				  AND grants.organization_id = media_materials.virtual_ips.organization_id
				  AND grants.team_id IN ?
			)
		)`, q.CallerID, q.CallerTeamIDs)
	} else {
		tx = tx.Where("owner_user_id = ? OR visibility = 'public'", q.CallerID)
	}
	var total int64
	err := tx.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	var rows []model.VirtualIp
	err = tx.Order("updated_at DESC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.VirtualIp, len(rows))
	for i := range rows {
		out[i] = toDomain(&rows[i])
	}
	return out, int(total), nil
}

func (r *Repo) Update(ctx context.Context, v *domain.VirtualIp) error {
	return dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.VirtualIp{}).
		Where("id = ? AND organization_id = ? AND deleted_at IS NULL", v.ID, v.OrganizationID).
		Updates(map[string]any{
			"name": v.Name, "description": v.Description,
			"avatar_object_path": v.AvatarObjectPath, "full_body_object_path": v.FullBodyObjectPath,
			"three_view_object_path": v.ThreeViewObjectPath, "nine_view_object_path": v.NineViewObjectPath,
			"persona_profile": v.PersonaProfile, "status": string(v.Status),
			"visibility": string(v.Visibility), "updated_at": time.Now(),
		}).Error
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	res := dal.FromContext(ctx, r.db).WithContext(ctx).Model(&model.VirtualIp{}).
		Where("id = ? AND deleted_at IS NULL", id).Update("deleted_at", time.Now())
	err := res.Error
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
		ID: v.ID, OrganizationID: v.OrganizationID, Scope: v.Scope, Name: v.Name, Description: v.Description,
		AvatarObjectPath: v.AvatarObjectPath, FullBodyObjectPath: v.FullBodyObjectPath,
		ThreeViewObjectPath: v.ThreeViewObjectPath, NineViewObjectPath: v.NineViewObjectPath,
		PersonaProfile: v.PersonaProfile,
		Status:         string(v.Status), Visibility: string(v.Visibility), OwnerUserID: v.OwnerUserID,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

func toDomain(row *model.VirtualIp) *domain.VirtualIp {
	return &domain.VirtualIp{
		ID: row.ID, OrganizationID: row.OrganizationID, Scope: row.Scope, Name: row.Name, Description: row.Description,
		AvatarObjectPath: row.AvatarObjectPath, FullBodyObjectPath: row.FullBodyObjectPath,
		ThreeViewObjectPath: row.ThreeViewObjectPath, NineViewObjectPath: row.NineViewObjectPath,
		PersonaProfile: row.PersonaProfile,
		Status:         domain.Status(row.Status), Visibility: domain.Visibility(row.Visibility),
		OwnerUserID: row.OwnerUserID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// —— GrantRepo ——

type GrantRepo struct{ db *gorm.DB }

func NewGrantRepo(db *gorm.DB) *GrantRepo { return &GrantRepo{db: db} }

func (r *GrantRepo) AddGrant(ctx context.Context, g *domain.Grant) error {
	row := &model.VirtualIpGrant{
		OrganizationID: g.OrganizationID, VirtualIPID: g.VirtualIPID, TeamID: g.TeamID,
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
			OrganizationID: rows[i].OrganizationID, VirtualIPID: rows[i].VirtualIPID, TeamID: rows[i].TeamID,
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
			OrganizationID: rows[i].OrganizationID, VirtualIPID: rows[i].VirtualIPID, TeamID: rows[i].TeamID,
			Permission: domain.Permission(rows[i].Permission), GrantedBy: rows[i].GrantedBy,
			GrantedAt: rows[i].GrantedAt,
		}
	}
	return out, nil
}
