// Package auth 是 auth 领域仓储的 dal 实现（users 表）。
// 实现 biz/domain/auth.UserRepo；唯一可 import biz/dal/query 的位置。
// 把 DB row model 转为 domain 对象，禁止把 GORM row model 返回给上层。
//
// 事务：所有方法经 dal.FromContext(ctx, db) 取连接，自动加入上层事务（不开自身事务）。
package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"media_agent/media_auth/biz/dal"
	"media_agent/media_auth/biz/dal/model"
	"media_agent/media_auth/biz/dal/query"
	domainauth "media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/ids"
)

// NewRepo 构造 auth 仓储。db 为 dal 层 *gorm.DB（非事务时用，事务时由 ctx 覆盖）。
func NewRepo(db *gorm.DB) domainauth.UserRepo {
	return &repo{db: db}
}

type repo struct {
	db *gorm.DB
}

// q 返回绑定到 ctx 连接（事务感知）的 query.Query。
func (r *repo) q(ctx context.Context) *query.Query {
	return query.Use(dal.FromContext(ctx, r.db))
}

// FindByUsername 按用户名查用户。不存在返回 ErrUserNotFound。
func (r *repo) FindByUsername(ctx context.Context, username string) (domainauth.User, error) {
	u, err := r.q(ctx).User.WithContext(ctx).Where(r.q(ctx).User.Username.Eq(username)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainauth.User{}, domainauth.ErrUserNotFound
		}
		return domainauth.User{}, err
	}
	return rowToDomain(u), nil
}

// FindByID 按 id 查用户。不存在返回 ErrUserNotFound。
func (r *repo) FindByID(ctx context.Context, id string) (domainauth.User, error) {
	u, err := r.q(ctx).User.WithContext(ctx).Where(r.q(ctx).User.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainauth.User{}, domainauth.ErrUserNotFound
		}
		return domainauth.User{}, err
	}
	return rowToDomain(u), nil
}

// Upsert 创建或更新用户（LDAP 登录命中时自动建号；已存在更新 display_name/email/last_login_at）。
// 对齐 control-plane upsertUser。
func (r *repo) Upsert(ctx context.Context, username, displayName, email string) (domainauth.User, error) {
	q := r.q(ctx)
	// 先查是否存在（按 username）。
	existing, err := q.User.WithContext(ctx).Where(q.User.Username.Eq(username)).First()
	if err == nil {
		// 已存在：更新 display_name/email/last_login_at。
		now := time.Now()
		updates := map[string]any{
			"display_name":  displayName,
			"last_login_at": now,
		}
		if email != "" {
			updates["email"] = email
		}
		if _, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(existing.ID)).Updates(updates); err != nil {
			return domainauth.User{}, err
		}
		existing.DisplayName = displayName
		existing.LastLoginAt = &now
		if email != "" {
			existing.Email = &email
		}
		return rowToDomain(existing), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domainauth.User{}, err
	}
	// 不存在：新建（password_hash 空 = LDAP/企微派生账号）。
	row := model.User{
		ID:          ids.New(),
		Username:    username,
		DisplayName: displayName,
		Email:       strPtr(email),
		Role:        "user",
	}
	if err := q.User.WithContext(ctx).Create(&row); err != nil {
		return domainauth.User{}, err
	}
	return rowToDomain(&row), nil
}

// CreateUser 新建用户（Register / admin 创建）。username 唯一冲突 -> ErrUsernameTaken。
// role 默认 user；passwordHash 空串 = 无本地密码（LDAP/企微派生）。
func (r *repo) CreateUser(ctx context.Context, username, displayName, passwordHash, email, role string) (domainauth.User, error) {
	q := r.q(ctx)
	if role == "" {
		role = "user"
	}
	row := model.User{
		ID:           ids.New(),
		Username:     username,
		DisplayName:  displayName,
		PasswordHash: strPtr(passwordHash),
		Email:        strPtr(email),
		Role:         role,
	}
	if err := q.User.WithContext(ctx).Omit(q.User.CreatedAt).Create(&row); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return domainauth.User{}, domainauth.ErrUsernameTaken
		}
		return domainauth.User{}, err
	}
	// 回读 DB 默认 created_at。
	return r.FindByID(ctx, row.ID)
}

// userSortColumns 白名单：sort 参数 -> 列名（防注入）。
var userSortColumns = map[string]string{
	"created":     "created_at",
	"name":        "display_name",
	"last_active": "last_login_at",
}

// ListUsers 分页+模糊搜索用户（排除 system）。返回 (users, total)。
//
// 用 raw gorm.DB 而非 gen：gen 的 Or 不便分组（A AND (B OR C OR D)），
// GORM 每个 Where 调用自动加括号，正好表达。
func (r *repo) ListUsers(ctx context.Context, q, sort, order string, page, pageSize int) ([]domainauth.User, int, error) {
	gormDB := dal.FromContext(ctx, r.db).Model(&model.User{})
	gormDB = gormDB.Where("role <> 'system'")
	if q != "" {
		like := "%" + q + "%"
		gormDB = gormDB.Where("username ILIKE ? OR display_name ILIKE ? OR email ILIKE ?", like, like, like)
	}

	var total int64
	if err := gormDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// sort 白名单（防注入）；order 仅 asc/desc。
	col, ok := userSortColumns[sort]
	if !ok {
		col = "created_at"
	}
	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	var rows []*model.User
	if err := gormDB.Order(col + " " + direction + " NULLS LAST").
		Offset(offset).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]domainauth.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToDomain(row))
	}
	return out, int(total), nil
}

// DeleteUser 按 id 删 user（FK CASCADE 清关联表）。0 行 -> ErrUserNotFound。
func (r *repo) DeleteUser(ctx context.Context, id string) error {
	q := r.q(ctx)
	res, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).Delete()
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainauth.ErrUserNotFound
	}
	return nil
}

// SetPassword 更新 password_hash（改密）。
func (r *repo) SetPassword(ctx context.Context, id, passwordHash string) error {
	q := r.q(ctx)
	res, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).UpdateColumn(q.User.PasswordHash, passwordHash)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainauth.ErrUserNotFound
	}
	return nil
}

// SetDefaultPrompt 更新 default_prompt_id（promptID 空串=清除 → NULL）。
func (r *repo) SetDefaultPrompt(ctx context.Context, id, promptID string) error {
	q := r.q(ctx)
	val := (*string)(nil)
	if promptID != "" {
		v := promptID
		val = &v
	}
	res, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).UpdateColumn(q.User.DefaultPromptID, val)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainauth.ErrUserNotFound
	}
	return nil
}

// SetAutoEvolution 更新 auto_evolution。
func (r *repo) SetAutoEvolution(ctx context.Context, id string, autoEvolution bool) error {
	q := r.q(ctx)
	res, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).UpdateColumn(q.User.AutoEvolution, autoEvolution)
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainauth.ErrUserNotFound
	}
	return nil
}

// UpdateLastLogin 更新 last_login_at = now（本地登录成功后）。
func (r *repo) UpdateLastLogin(ctx context.Context, id string) error {
	q := r.q(ctx)
	res, err := q.User.WithContext(ctx).Where(q.User.ID.Eq(id)).UpdateColumn(q.User.LastLoginAt, time.Now())
	if err != nil {
		return err
	}
	if res.RowsAffected == 0 {
		return domainauth.ErrUserNotFound
	}
	return nil
}

// rowToDomain 把 GORM row model 转为 domain User（不泄漏 row model）。
func rowToDomain(u *model.User) domainauth.User {
	d := domainauth.User{
		ID:              u.ID,
		Username:        u.Username,
		DisplayName:     u.DisplayName,
		Role:            u.Role,
		DefaultPromptID: ptrStr(u.DefaultPromptID),
		AutoEvolution:   u.AutoEvolution,
		CreatedAt:       u.CreatedAt,
		PasswordHash:    ptrStr(u.PasswordHash),
	}
	if u.Email != nil {
		d.Email = *u.Email
	}
	if u.LastLoginAt != nil {
		d.LastLoginAt = *u.LastLoginAt
	}
	return d
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
