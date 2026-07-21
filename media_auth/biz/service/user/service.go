// Package user 是用户管理用例服务层（admin 鉴权）。
//
// 单 domain（auth）编排：ListUsers / CreateUser / DeleteUser / ChangeUserPassword。
// 所有方法要求 role==admin（非 admin -> ErrForbidden）。
// 禁止 import Hertz / hertz_gen / biz/dal / 其它 service 包（同层隔离）。
package user

import (
	"context"
	"errors"

	domainauth "media_agent/media_auth/biz/domain/auth"
	policyauth "media_agent/media_auth/biz/policy/auth"
	"media_agent/media_auth/biz/shared/argon2"
)

// Service 是用户管理用例服务。
type Service struct {
	repo domainauth.UserRepo
}

// NewService 构造用户管理服务。
func NewService(repo domainauth.UserRepo) *Service {
	return &Service{repo: repo}
}

// UserView 是 service 层返回的用户记录（与 DTO 对齐）。
type UserView struct {
	ID          string
	Username    string
	DisplayName string
	Email       string
	Role        string
	AuthSource  string // password | ldap（admin 列表仅按 password_hash 推导，不解析 wecom）
	CreatedAt   string // RFC3339
	LastLoginAt string // RFC3339，空=从未登录
}

// 用例错误。
var (
	ErrForbidden     = errors.New("admin only")
	ErrUserNotFound  = domainauth.ErrUserNotFound
	ErrUsernameTaken = domainauth.ErrUsernameTaken
)

// ListUsers 分页+搜索用户（admin only）。返回 (views, total)。
func (s *Service) ListUsers(ctx context.Context, role, q, sort, order string, page, pageSize int) ([]UserView, int, error) {
	if role != "admin" {
		return nil, 0, ErrForbidden
	}
	users, total, err := s.repo.ListUsers(ctx, q, sort, order, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]UserView, 0, len(users))
	for _, u := range users {
		out = append(out, toUserView(u))
	}
	return out, total, nil
}

// CreateUser 新建用户（admin only）。role 可选 user|admin，默认 user。
func (s *Service) CreateUser(ctx context.Context, role string, cmd CreateUserCommand) (UserView, error) {
	if role != "admin" {
		return UserView{}, ErrForbidden
	}
	if err := policyauth.ValidateCreateUser(cmd.Username, cmd.Password); err != nil {
		return UserView{}, err
	}
	targetRole := "user"
	if cmd.Role != nil && *cmd.Role != "" {
		if !policyauth.IsValidRole(*cmd.Role) {
			return UserView{}, policyauth.ErrInvalidRole
		}
		targetRole = *cmd.Role
	}
	displayName := cmd.Username
	if cmd.DisplayName != "" {
		displayName = cmd.DisplayName
	}
	hash, err := argon2.Hash(cmd.Password)
	if err != nil {
		return UserView{}, err
	}
	u, err := s.repo.CreateUser(ctx, cmd.Username, displayName, hash, cmd.Email, targetRole)
	if err != nil {
		return UserView{}, err
	}
	return toUserView(u), nil
}

// DeleteUser 删 user（admin only；不能删自己）。FK CASCADE 清关联表。
func (s *Service) DeleteUser(ctx context.Context, role, currentUserID, targetID string) error {
	if role != "admin" {
		return ErrForbidden
	}
	if err := policyauth.CanDeleteUser(targetID, currentUserID); err != nil {
		return err
	}
	return s.repo.DeleteUser(ctx, targetID)
}

// ChangeUserPassword admin 改他人密码（admin only；仅本地密码账号）。
func (s *Service) ChangeUserPassword(ctx context.Context, role, targetID, password string) error {
	if role != "admin" {
		return ErrForbidden
	}
	if len(password) < 6 {
		return policyauth.ErrPasswordTooShort
	}
	target, err := s.repo.FindByID(ctx, targetID)
	if err != nil {
		return err // ErrUserNotFound
	}
	if !policyauth.CanChangePassword(target) {
		return ErrPasswordNotSet
	}
	hash, err := argon2.Hash(password)
	if err != nil {
		return err
	}
	return s.repo.SetPassword(ctx, targetID, hash)
}

// ErrPasswordNotSet 目标账号无本地密码（LDAP/企微派生），不可改密。
var ErrPasswordNotSet = errors.New("account has no local password")

// CreateUserCommand 是 CreateUser 用例入参。
type CreateUserCommand struct {
	Username    string
	DisplayName string
	Password    string
	Email       string
	Role        *string // nil/空=user；user|admin
}

// toUserView 把 domain User 转 UserView。auth_source 按 password_hash 推导（password|ldap）。
func toUserView(u domainauth.User) UserView {
	v := UserView{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		Role:        u.Role,
		AuthSource:  u.AuthSource(nil), // nil resolver -> password|ldap
		CreatedAt:   u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if !u.LastLoginAt.IsZero() {
		v.LastLoginAt = u.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return v
}
