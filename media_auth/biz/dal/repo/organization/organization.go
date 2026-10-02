package organization

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	domain "media_agent/media_auth/biz/domain/organization"
	"media_agent/media_auth/biz/shared/ids"
)

type repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) domain.Repository       { return &repo{db: db} }
func (r *repo) conn(ctx context.Context) *gorm.DB { return dal.FromContext(ctx, r.db).WithContext(ctx) }

func (r *repo) ListForUser(ctx context.Context, userID string) ([]domain.Summary, error) {
	var rows []struct {
		model.Organization
		MyRole string `gorm:"column:my_role"`
	}
	err := r.conn(ctx).Table("media_auth.organizations o").
		Select("o.*, om.role AS my_role").
		Joins("JOIN media_auth.organization_members om ON om.organization_id=o.id").
		Where("om.user_id = ?", userID).Order("o.created_at").Scan(&rows).Error
	out := make([]domain.Summary, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Summary{Organization: toDomain(row.Organization), MyRole: row.MyRole})
	}
	return out, err
}

func (r *repo) Create(ctx context.Context, name, creatorID string) (domain.Summary, error) {
	var out domain.Summary
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.Organization{ID: ids.New(), Name: name, Status: "active", Version: 1}
		if err := tx.Omit("created_at", "updated_at").Create(&row).Error; err != nil {
			return err
		}
		member := model.OrganizationMember{OrganizationID: row.ID, UserID: creatorID, Role: "admin"}
		if err := tx.Omit("joined_at").Create(&member).Error; err != nil {
			return err
		}
		if err := enqueue(tx, row.ID, 1, "OrganizationCreated", map[string]any{"organization_id": row.ID, "admin_user_id": creatorID}); err != nil {
			return err
		}
		if err := tx.First(&row, "id = ?", row.ID).Error; err != nil {
			return err
		}
		out = domain.Summary{Organization: toDomain(row), MyRole: "admin"}
		return nil
	})
	return out, err
}

func (r *repo) Get(ctx context.Context, id string) (domain.Organization, error) {
	var row model.Organization
	if err := r.conn(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Organization{}, domain.ErrNotFound
		}
		return domain.Organization{}, err
	}
	return toDomain(row), nil
}

func (r *repo) Update(ctx context.Context, id string, name, status *string) (domain.Organization, error) {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"version": gorm.Expr("version + 1"), "updated_at": time.Now()}
		if name != nil {
			updates["name"] = *name
		}
		if status != nil {
			updates["status"] = *status
		}
		res := tx.Model(&model.Organization{}).Where("id = ?", id).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotFound
		}
		var row model.Organization
		if err := tx.First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		return enqueue(tx, id, row.Version, "OrganizationChanged", map[string]any{"organization_id": id, "status": row.Status})
	})
	if err != nil {
		return domain.Organization{}, err
	}
	return r.Get(ctx, id)
}

func (r *repo) GetMembership(ctx context.Context, organizationID, userID string) (domain.Membership, error) {
	var row struct {
		model.OrganizationMember
		UserName string `gorm:"column:user_name"`
	}
	err := r.conn(ctx).Table("media_auth.organization_members om").Select("om.*, COALESCE(u.display_name,u.username) AS user_name").
		Joins("JOIN media_auth.users u ON u.id=om.user_id").Where("om.organization_id=? AND om.user_id=?", organizationID, userID).Scan(&row).Error
	if err != nil {
		return domain.Membership{}, err
	}
	if row.UserID == "" {
		return domain.Membership{}, domain.ErrNotMember
	}
	return membership(row.OrganizationMember, row.UserName), nil
}

func (r *repo) ListMembers(ctx context.Context, id string) ([]domain.Membership, error) {
	var rows []struct {
		model.OrganizationMember
		UserName string `gorm:"column:user_name"`
	}
	err := r.conn(ctx).Table("media_auth.organization_members om").Select("om.*, COALESCE(u.display_name,u.username) AS user_name").
		Joins("JOIN media_auth.users u ON u.id=om.user_id").Where("om.organization_id=?", id).Order("om.joined_at").Scan(&rows).Error
	out := make([]domain.Membership, 0, len(rows))
	for _, row := range rows {
		out = append(out, membership(row.OrganizationMember, row.UserName))
	}
	return out, err
}

func (r *repo) PutMember(ctx context.Context, organizationID, userID, role string) (domain.Membership, error) {
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.OrganizationMember{OrganizationID: organizationID, UserID: userID, Role: role}
		if err := tx.Omit("joined_at").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"role": role})}).Create(&row).Error; err != nil {
			return err
		}
		var org model.Organization
		if err := tx.First(&org, "id=?", organizationID).Error; err != nil {
			return err
		}
		if err := tx.Model(&org).Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return enqueue(tx, organizationID, org.Version+1, "OrganizationMembershipChanged", map[string]any{"organization_id": organizationID, "user_id": userID, "role": role})
	})
	if err != nil {
		return domain.Membership{}, err
	}
	return r.GetMembership(ctx, organizationID, userID)
}

func (r *repo) RemoveMember(ctx context.Context, organizationID, userID string) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.OrganizationMember{}, "organization_id=? AND user_id=?", organizationID, userID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrNotMember
		}
		var org model.Organization
		if err := tx.First(&org, "id=?", organizationID).Error; err != nil {
			return err
		}
		if err := tx.Model(&org).UpdateColumns(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return enqueue(tx, organizationID, org.Version+1, "OrganizationMembershipChanged", map[string]any{"organization_id": organizationID, "user_id": userID, "removed": true})
	})
}

func (r *repo) CountAdmins(ctx context.Context, id string) (int, error) {
	var n int64
	err := r.conn(ctx).Model(&model.OrganizationMember{}).Where("organization_id=? AND role='admin'", id).Count(&n).Error
	return int(n), err
}
func (r *repo) UserExists(ctx context.Context, id string) (bool, error) {
	var n int64
	err := r.conn(ctx).Model(&model.User{}).Where("id=? AND status='active'", id).Count(&n).Error
	return n > 0, err
}
func (r *repo) GetEntitlements(ctx context.Context, id string) ([]string, error) {
	var keys []string
	err := r.conn(ctx).Model(&model.OrganizationEntitlement{}).Where("organization_id=?", id).Order("feature_key").Pluck("feature_key", &keys).Error
	return keys, err
}

func (r *repo) SetEntitlements(ctx context.Context, id, grantedBy string, keys []string) ([]string, error) {
	keys = unique(keys)
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("organization_id=?", id).Delete(&model.OrganizationEntitlement{}).Error; err != nil {
			return err
		}
		for _, key := range keys {
			row := model.OrganizationEntitlement{OrganizationID: id, FeatureKey: key, GrantedBy: grantedBy}
			if err := tx.Omit("granted_at").Create(&row).Error; err != nil {
				return err
			}
		}
		var org model.Organization
		if err := tx.First(&org, "id=?", id).Error; err != nil {
			return err
		}
		if err := tx.Model(&org).UpdateColumns(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return enqueue(tx, id, org.Version+1, "OrganizationEntitlementsChanged", map[string]any{"organization_id": id, "feature_keys": keys})
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func enqueue(tx *gorm.DB, aggregateID string, sequence int64, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := tx.Exec("INSERT INTO media_auth.authorization_outbox (id,aggregate_type,aggregate_id,event_type,payload,sequence) VALUES (?,?,?,?,?::jsonb,?)", ids.New(), "organization", aggregateID, eventType, string(b), sequence).Error; err != nil {
		return err
	}
	rows, err := tx.Raw("UPDATE media_auth.teams SET authorization_version = authorization_version + 1, updated_at = now() WHERE organization_id = ? RETURNING id, authorization_version", aggregateID).Rows()
	if err != nil {
		return err
	}
	// Consume and close the cursor before issuing another statement on this
	// transaction. pgx cannot execute while the previous result set is open.
	type changedTeam struct {
		id      string
		version int64
	}
	changed := make([]changedTeam, 0)
	for rows.Next() {
		var teamID string
		var teamVersion int64
		if err := rows.Scan(&teamID, &teamVersion); err != nil {
			rows.Close()
			return err
		}
		changed = append(changed, changedTeam{id: teamID, version: teamVersion})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, team := range changed {
		teamPayload, err := json.Marshal(map[string]any{"team_id": team.id, "organization_id": aggregateID})
		if err != nil {
			return err
		}
		if err := tx.Exec("INSERT INTO media_auth.authorization_outbox (id,aggregate_type,aggregate_id,event_type,payload,sequence) VALUES (?,?,?,?,?::jsonb,?)", ids.New(), "team", team.id, "TeamOrganizationAuthorizationChanged", string(teamPayload), team.version).Error; err != nil {
			return err
		}
	}
	return nil
}
func toDomain(r model.Organization) domain.Organization {
	return domain.Organization{ID: r.ID, Name: r.Name, Status: r.Status, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func membership(r model.OrganizationMember, name string) domain.Membership {
	return domain.Membership{OrganizationID: r.OrganizationID, UserID: r.UserID, UserName: name, Role: r.Role, JoinedAt: r.JoinedAt}
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
