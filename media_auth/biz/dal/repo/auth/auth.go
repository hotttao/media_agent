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
