// Package team 是 team 领域仓储的 dal 实现（teams / team_members / team_invites 三表）。
// 实现 biz/domain/team.TeamRepo；唯一可 import biz/dal/query 的位置。
// 把 DB row model 转为 domain 对象，禁止把 GORM row model 返回给上层。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
// 成员展示名（display_name）与 user 存在性检查经 q.User 只读查询（dal/repo 可用全表 query）。
package team

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainteam "media_agent/media_auth/biz/domain/team"
	"media_agent/media_auth/biz/shared/ids"
)

// NewRepo 构造 team 仓储。db 为 dal 层 *gorm.DB（非事务时用，事务时由 ctx 覆盖）。
func NewRepo(db *gorm.DB) domainteam.TeamRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

// q 返回绑定到 ctx 连接（事务感知）的 query.Query。
func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

func (r *repo) conn(ctx context.Context) *gorm.DB {
	return dal.FromContext(ctx, r.db).WithContext(ctx)
}

// InsertTeam 仅插 team 行。description 空串 → NULL。
func (r *repo) InsertTeam(ctx context.Context, organizationID, createdBy, name, description string) (domainteam.Team, error) {
	row := model.Team{
		ID:                   ids.New(),
		OrganizationID:       organizationID,
		Name:                 name,
		Description:          strPtr(description),
		EntitlementMode:      "inherit",
		Status:               "active",
		AuthorizationVersion: 1,
		CreatedBy:            createdBy,
	}
	if err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return enqueueTeam(tx, row.ID, "TeamCreated", map[string]any{"team_id": row.ID}, false)
	}); err != nil {
		return domainteam.Team{}, err
	}
	return rowToTeam(&row), nil
}

// GetTeam 按 id 查 team。不存在返回 ErrTeamNotFound。
func (r *repo) GetTeam(ctx context.Context, id string) (domainteam.Team, error) {
	q := r.q(ctx)
	t, err := q.Team.WithContext(ctx).Where(q.Team.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainteam.Team{}, domainteam.ErrTeamNotFound
		}
		return domainteam.Team{}, err
	}
	return rowToTeam(t), nil
}

// UpdateTeam 局部更新 name/description（nil 跳过）。0 行返回 ErrTeamNotFound。
//
// description tri-state：nil=不改；&""=清空(NULL)；&"x"=设置。
func (r *repo) UpdateTeam(ctx context.Context, id string, patch domainteam.TeamPatch) (domainteam.Team, error) {
	q := r.q(ctx)
	updates := map[string]any{}
	if patch.Name != nil {
		updates["name"] = *patch.Name
	}
	if patch.Description != nil {
		// &"" → NULL；&"x" → 值。GORM map 更新无法直接表达 NULL，用 nil 表达 NULL。
		if *patch.Description == "" {
			updates["description"] = nil
		} else {
			updates["description"] = *patch.Description
		}
	}
	if len(updates) == 0 {
		// 无字段更新：直接回读。
		return r.GetTeam(ctx, id)
	}
	updates["updated_at"] = time.Now()
	res, err := q.Team.WithContext(ctx).Where(q.Team.ID.Eq(id)).Updates(updates)
	if err != nil {
		return domainteam.Team{}, err
	}
	if res.RowsAffected == 0 {
		return domainteam.Team{}, domainteam.ErrTeamNotFound
	}
	return r.GetTeam(ctx, id)
}

// DeleteTeam 删 team（级联清 team_members + team_invites）。0 行返回 ErrTeamNotFound。
func (r *repo) DeleteTeam(ctx context.Context, id string) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		if err := enqueueTeam(tx, id, "TeamDeleted", map[string]any{"team_id": id}, true); err != nil {
			return err
		}
		res := tx.Delete(&model.Team{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domainteam.ErrTeamNotFound
		}
		return nil
	})
}

func (r *repo) GetEntitlements(ctx context.Context, id string) ([]string, error) {
	var keys []string
	err := r.conn(ctx).Model(&model.TeamEntitlement{}).
		Where("team_id = ?", id).Order("feature_key").Pluck("feature_key", &keys).Error
	return keys, err
}

func (r *repo) SetEntitlements(ctx context.Context, id, grantedBy, mode string, keys []string) ([]string, error) {
	keys = uniqueStrings(keys)
	if mode == "inherit" {
		keys = nil
	}
	err := r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Team{}).Where("id = ?", id).Update("entitlement_mode", mode)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domainteam.ErrTeamNotFound
		}
		if err := tx.Where("team_id = ?", id).Delete(&model.TeamEntitlement{}).Error; err != nil {
			return err
		}
		for _, key := range keys {
			row := model.TeamEntitlement{TeamID: id, FeatureKey: key, GrantedBy: grantedBy}
			if err := tx.Omit("granted_at").Create(&row).Error; err != nil {
				return err
			}
		}
		return enqueueTeam(tx, id, "TeamEntitlementsChanged", map[string]any{"team_id": id, "entitlement_mode": mode, "feature_keys": keys}, true)
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// ListTeamsForUser 查 user 所属全部 team，含 my_role + member_count。
func (r *repo) ListTeamsForUser(ctx context.Context, userID string) ([]domainteam.TeamSummary, error) {
	q := r.q(ctx)
	// 先查 user 的全部成员关系（含 role），再逐 team 取行 + member_count。
	members, err := q.TeamMember.WithContext(ctx).Where(q.TeamMember.UserID.Eq(userID)).Find()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []domainteam.TeamSummary{}, nil
	}
	teamIDs := make([]string, 0, len(members))
	roleByTeam := make(map[string]domainteam.Role, len(members))
	for _, m := range members {
		teamIDs = append(teamIDs, m.TeamID)
		roleByTeam[m.TeamID] = domainteam.Role(m.Role)
	}
	teams, err := q.Team.WithContext(ctx).Where(q.Team.ID.In(teamIDs...)).Order(q.Team.CreatedAt.Desc()).Find()
	if err != nil {
		return nil, err
	}
	// 统计每个 team 的成员数：一次取全部相关成员行，Go 内按 team_id 计数。
	allMembers, err := q.TeamMember.WithContext(ctx).Where(q.TeamMember.TeamID.In(teamIDs...)).Find()
	if err != nil {
		return nil, err
	}
	countByTeam := make(map[string]int, len(teamIDs))
	for _, m := range allMembers {
		countByTeam[m.TeamID]++
	}
	out := make([]domainteam.TeamSummary, 0, len(teams))
	for _, t := range teams {
		out = append(out, domainteam.TeamSummary{
			Team:        rowToTeam(t),
			MyRole:      roleByTeam[t.ID],
			MemberCount: countByTeam[t.ID],
		})
	}
	return out, nil
}

// GetMembership 查 user 在 team 的成员关系。不存在返回 ErrNotMember。
func (r *repo) GetMembership(ctx context.Context, teamID, userID string) (domainteam.Membership, error) {
	q := r.q(ctx)
	m, err := q.TeamMember.WithContext(ctx).
		Where(q.TeamMember.TeamID.Eq(teamID), q.TeamMember.UserID.Eq(userID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainteam.Membership{}, domainteam.ErrNotMember
		}
		return domainteam.Membership{}, err
	}
	return rowToMembership(m), nil
}

// ListMembers 列 team 全部成员（JOIN users 取 display_name，按 joined_at ASC）。
func (r *repo) ListMembers(ctx context.Context, teamID string) ([]domainteam.MemberWithUser, error) {
	q := r.q(ctx)
	members, err := q.TeamMember.WithContext(ctx).
		Where(q.TeamMember.TeamID.Eq(teamID)).Order(q.TeamMember.JoinedAt).Find()
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []domainteam.MemberWithUser{}, nil
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}
	users, err := q.User.WithContext(ctx).Where(q.User.ID.In(userIDs...)).Find()
	if err != nil {
		return nil, err
	}
	nameByID := make(map[string]string, len(users))
	for _, u := range users {
		nameByID[u.ID] = displayOrUsername(u.DisplayName, u.Username)
	}
	out := make([]domainteam.MemberWithUser, 0, len(members))
	for _, m := range members {
		out = append(out, domainteam.MemberWithUser{
			Membership: rowToMembership(m),
			UserName:   nameByID[m.UserID],
		})
	}
	return out, nil
}

// CountAdmins 统计 team 当前 admin 数量。
func (r *repo) CountAdmins(ctx context.Context, teamID string) (int, error) {
	q := r.q(ctx)
	count, err := q.TeamMember.WithContext(ctx).
		Where(q.TeamMember.TeamID.Eq(teamID), q.TeamMember.Role.Eq(string(domainteam.RoleAdmin))).Count()
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// AddMember 加入成员（ON CONFLICT DO NOTHING 幂等）。
// Omit JoinedAt 让 DB DEFAULT now() 生效（避免 GORM 写入零值时间）。
func (r *repo) AddMember(ctx context.Context, teamID, userID string, role domainteam.Role) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.TeamMember{TeamID: teamID, UserID: userID, Role: string(role)}
		res := tx.Omit("joined_at").Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil || res.RowsAffected == 0 {
			return res.Error
		}
		return enqueueTeam(tx, teamID, "TeamMembershipChanged", map[string]any{"team_id": teamID, "user_id": userID, "role": role}, true)
	})
}

// UpdateMemberRole 改成员 role。0 行返回 ErrMemberNotFound。
func (r *repo) UpdateMemberRole(ctx context.Context, teamID, userID string, role domainteam.Role) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.TeamMember{}).Where("team_id = ? AND user_id = ?", teamID, userID).Update("role", string(role))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domainteam.ErrMemberNotFound
		}
		return enqueueTeam(tx, teamID, "TeamMembershipChanged", map[string]any{"team_id": teamID, "user_id": userID, "role": role}, true)
	})
}

// RemoveMember 移除成员。0 行返回 ErrMemberNotFound。
func (r *repo) RemoveMember(ctx context.Context, teamID, userID string) error {
	return r.conn(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Delete(&model.TeamMember{}, "team_id = ? AND user_id = ?", teamID, userID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domainteam.ErrMemberNotFound
		}
		return enqueueTeam(tx, teamID, "TeamMembershipChanged", map[string]any{"team_id": teamID, "user_id": userID, "removed": true}, true)
	})
}

// UserExists 判定 user 是否存在。
func (r *repo) UserExists(ctx context.Context, userID string) (bool, error) {
	q := r.q(ctx)
	count, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(userID)).Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UserDisplayName 取 user 展示名。不存在返回 ""。
func (r *repo) UserDisplayName(ctx context.Context, userID string) (string, error) {
	q := r.q(ctx)
	u, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(userID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return displayOrUsername(u.DisplayName, u.Username), nil
}

// CreateInvite 签发邀请（token = tinv_ + base64url(rand 24)）。
func (r *repo) CreateInvite(ctx context.Context, teamID, createdBy string, expiresAt *time.Time) (domainteam.Invite, error) {
	q := r.q(ctx)
	token, err := newInviteToken()
	if err != nil {
		return domainteam.Invite{}, err
	}
	row := model.TeamInvite{
		Token:     token,
		TeamID:    teamID,
		CreatedBy: createdBy,
		ExpiresAt: expiresAt,
	}
	if err := q.TeamInvite.WithContext(ctx).Create(&row); err != nil {
		return domainteam.Invite{}, err
	}
	return rowToInvite(&row), nil
}

// GetInviteByToken 按 token 查邀请。不存在返回 ErrInviteNotFound。
func (r *repo) GetInviteByToken(ctx context.Context, token string) (domainteam.Invite, error) {
	q := r.q(ctx)
	inv, err := q.TeamInvite.WithContext(ctx).Where(q.TeamInvite.Token.Eq(token)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainteam.Invite{}, domainteam.ErrInviteNotFound
		}
		return domainteam.Invite{}, err
	}
	return rowToInvite(inv), nil
}

// ListInvites 列 team 未过期邀请（expires_at IS NULL OR > now）。
//
// gen 的 field.Expr 不支持 .Or 组合；这里取 team 全部邀请后在 Go 内按 policy.IsExpired 过滤。
// 单 team 邀请数量有限，内存过滤可接受。
func (r *repo) ListInvites(ctx context.Context, teamID string) ([]domainteam.Invite, error) {
	q := r.q(ctx)
	rows, err := q.TeamInvite.WithContext(ctx).
		Where(q.TeamInvite.TeamID.Eq(teamID)).
		Order(q.TeamInvite.CreatedAt.Desc()).Find()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]domainteam.Invite, 0, len(rows))
	for _, inv := range rows {
		d := rowToInvite(inv)
		if d.ExpiresAt != nil && !d.ExpiresAt.After(now) {
			continue // 已过期，跳过
		}
		out = append(out, d)
	}
	return out, nil
}

// DeleteInvite 按 token 撤销邀请。0 行返回 ErrInviteNotFound。
func (r *repo) DeleteInvite(ctx context.Context, token string) error {
	q := r.q(ctx)
	res, err := q.TeamInvite.WithContext(ctx).Where(q.TeamInvite.Token.Eq(token)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainteam.ErrInviteNotFound
	}
	return nil
}

// ── row → domain 转换 ────────────────────────────────────────────────

func rowToTeam(t *model.Team) domainteam.Team {
	d := domainteam.Team{
		ID:                   t.ID,
		OrganizationID:       t.OrganizationID,
		Name:                 t.Name,
		EntitlementMode:      t.EntitlementMode,
		Status:               t.Status,
		AuthorizationVersion: t.AuthorizationVersion,
		CreatedBy:            t.CreatedBy,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
	}
	if t.Description != nil {
		d.Description = *t.Description
	}
	return d
}

func rowToMembership(m *model.TeamMember) domainteam.Membership {
	return domainteam.Membership{
		TeamID:   m.TeamID,
		UserID:   m.UserID,
		Role:     domainteam.Role(m.Role),
		JoinedAt: m.JoinedAt,
	}
}

func rowToInvite(inv *model.TeamInvite) domainteam.Invite {
	return domainteam.Invite{
		Token:     inv.Token,
		TeamID:    inv.TeamID,
		CreatedBy: inv.CreatedBy,
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// displayOrUsername 优先 display_name，缺失回退 username。
func displayOrUsername(displayName, username string) string {
	if displayName != "" {
		return displayName
	}
	return username
}

// newInviteToken 生成 tinv_ + base64url(24 random bytes)。
func newInviteToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "tinv_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func enqueueTeam(tx *gorm.DB, teamID, eventType string, payload any, bump bool) error {
	var version int64
	if bump {
		row := tx.Raw("UPDATE media_auth.teams SET authorization_version = authorization_version + 1, updated_at = now() WHERE id = ? RETURNING authorization_version", teamID).Row()
		if err := row.Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domainteam.ErrTeamNotFound
			}
			return err
		}
	} else if err := tx.Model(&model.Team{}).Select("authorization_version").Where("id = ?", teamID).Scan(&version).Error; err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Exec("INSERT INTO media_auth.authorization_outbox (id,aggregate_type,aggregate_id,event_type,payload,sequence) VALUES (?,?,?,?,?::jsonb,?)", ids.New(), "team", teamID, eventType, string(body), version).Error
}
