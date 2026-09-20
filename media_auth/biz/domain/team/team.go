// Package team 存放 team（团队与成员、邀请）领域的稳定业务核心：Team 实体、
// Membership / Invite 值对象、Role 枚举、仓库接口、不变量与 domain 错误。
//
// 禁止 import Hertz / hertz_gen / biz/dal / biz/infra / logger / config / ORM。
// team 是 user 之上组织 user 的命名分组，仅作 Resource Grant ACL 基础数据；
// 不承担多租户隔离、不承担 billing（详见 docs/ddd/media_auth/domain/0007-team/team.md）。
package team

import (
	"context"
	"time"
)

// Role 是 team 内成员角色档位。
type Role string

const (
	RoleAdmin  Role = "admin"  // 可改 team / 增删成员 / 创建与撤销邀请
	RoleMember Role = "member" // 仅可查看与主动退团
)

// Team 是团队实体（对应 media_auth.teams 表的领域形态，非 GORM row）。
type Team struct {
	ID                   string
	OrganizationID       string
	Name                 string
	Description          string // 空串 = NULL
	EntitlementMode      string
	Status               string
	AuthorizationVersion int64
	CreatedBy            string // 创建者 user id（首任 admin）
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Membership 是 (team_id, user_id) → role 的成员关系值对象。
type Membership struct {
	TeamID   string
	UserID   string
	Role     Role
	JoinedAt time.Time
}

// MemberWithUser 是成员关系 + users.display_name（JOIN 后的展示形态）。
type MemberWithUser struct {
	Membership
	UserName string // users.display_name，缺失回退 username
}

// TeamSummary 是 ListMyTeams 返回的 team + 当前 user 的 role + 成员数。
type TeamSummary struct {
	Team
	MyRole      Role
	MemberCount int
}

// Invite 是邀请链接值对象（team_invites 行的领域形态）。
//
// ExpiresAt 为 nil 表示不过期；非 nil 由 policy.IsExpired 判定。
type Invite struct {
	Token     string
	TeamID    string
	CreatedBy string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// TeamPatch 是 UpdateTeam 的局部更新入参（tri-state：nil 指针 = 不改）。
//
// Description：nil=不改；&""=清空(NULL)；&"x"=设置。
type TeamPatch struct {
	Name        *string
	Description *string
}

// TeamRepo 是 team 领域的仓储接口（由 dal/repo/team 实现）。
//
// 仓储方法收 context.Context：上层在事务内时 ctx 携带 tx（dal.FromContext），
// repo 据此加入事务；不在事务内时用默认连接。仓储自身不开事务（CreateTeam 除外：
// 建 team + 首任 admin 须同事务，由 repo 内部 GORM Transaction 保证）。
type TeamRepo interface {
	// InsertTeam 仅插 team 行（不含成员）。返回落库后的 Team（含 id）。
	// 建 team + 首任 admin 的原子性由 service 在 TxRunner 内调 InsertTeam + AddMember 保证。
	InsertTeam(ctx context.Context, organizationID, createdBy, name, description string) (Team, error)
	// GetTeam 按 id 查 team。不存在返回 ErrTeamNotFound。
	GetTeam(ctx context.Context, id string) (Team, error)
	// UpdateTeam 局部更新 name/description（nil 跳过）。0 行返回 ErrTeamNotFound。
	UpdateTeam(ctx context.Context, id string, patch TeamPatch) (Team, error)
	// DeleteTeam 删 team（级联清 team_members + team_invites）。0 行返回 ErrTeamNotFound。
	DeleteTeam(ctx context.Context, id string) error
	GetEntitlements(ctx context.Context, id string) ([]string, error)
	SetEntitlements(ctx context.Context, id, grantedBy, mode string, keys []string) ([]string, error)
	// ListTeamsForUser 查 user 所属全部 team，含 my_role + member_count。
	ListTeamsForUser(ctx context.Context, userID string) ([]TeamSummary, error)

	// GetMembership 查 user 在 team 的成员关系。不存在返回 ErrNotMember。
	GetMembership(ctx context.Context, teamID, userID string) (Membership, error)
	// ListMembers 列 team 全部成员（JOIN users 取 display_name，按 joined_at ASC）。
	ListMembers(ctx context.Context, teamID string) ([]MemberWithUser, error)
	// CountAdmins 统计 team 当前 admin 数量（末位 admin 保护用）。
	CountAdmins(ctx context.Context, teamID string) (int, error)
	// AddMember 加入成员（ON CONFLICT DO NOTHING 幂等）。
	AddMember(ctx context.Context, teamID, userID string, role Role) error
	// UpdateMemberRole 改成员 role。0 行返回 ErrMemberNotFound。
	UpdateMemberRole(ctx context.Context, teamID, userID string, role Role) error
	// RemoveMember 移除成员。0 行返回 ErrMemberNotFound。
	RemoveMember(ctx context.Context, teamID, userID string) error

	// UserExists 判定 user 是否存在（AddMember 目标校验用）。
	UserExists(ctx context.Context, userID string) (bool, error)
	// UserDisplayName 取 user 展示名（inviter_name / member_name 用）。不存在返回 ""。
	UserDisplayName(ctx context.Context, userID string) (string, error)

	// CreateInvite 签发邀请（token 由 repo 生成）。返回落库后的 Invite。
	CreateInvite(ctx context.Context, teamID, createdBy string, expiresAt *time.Time) (Invite, error)
	// GetInviteByToken 按 token 查邀请。不存在返回 ErrInviteNotFound。
	GetInviteByToken(ctx context.Context, token string) (Invite, error)
	// ListInvites 列 team 未过期邀请（expires_at IS NULL OR > now）。
	ListInvites(ctx context.Context, teamID string) ([]Invite, error)
	// DeleteInvite 按 token 撤销邀请。0 行返回 ErrInviteNotFound。
	DeleteInvite(ctx context.Context, token string) error
}

// domain 错误。repo 实现用 errors.Is 透传，service/handler 据此映射 HTTP 状态。
var (
	ErrTeamNotFound   = errSentinel("team not found")
	ErrNotMember      = errSentinel("not a member")
	ErrMemberNotFound = errSentinel("member not found")
	ErrInviteNotFound = errSentinel("invite not found")
)

// errSentinel 是简单的哨兵错误类型，支持 errors.Is 比较。
type errSentinel string

func (e errSentinel) Error() string { return string(e) }
