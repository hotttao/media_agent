// command.go：team 用例的 Command 与 Result 类型（用例输入/输出，非生成 DTO）。
package team

import (
	"time"

	domainteam "media_agent/media_auth/biz/domain/team"
)

// CreateTeamCommand 是 CreateTeam 用例入参。Description 空串=不设。
type CreateTeamCommand struct {
	Name        string
	Description string
}

// UpdateTeamCommand 是 UpdateTeam 用例入参（tri-state：nil 指针 = 不改）。
type UpdateTeamCommand struct {
	Name        *string
	Description *string // &""=清空；&"x"=设置；nil=不改
}

// AddMemberCommand 是 AddMember 用例入参。Role nil=默认 member。
type AddMemberCommand struct {
	UserID string
	Role   *string
}

// CreateInviteCommand 是 CreateInvite 用例入参。ExpiresInDays nil=默认 7 天。
type CreateInviteCommand struct {
	ExpiresInDays *int
}

// TeamView 是带当前 user 视角的 team（GetTeam/CreateTeam/UpdateTeam 返回）。
type TeamView struct {
	domainteam.Team
	MyRole      domainteam.Role
	MemberCount int
}

// InviteView 是带创建者名的邀请（CreateInvite/ListInvites 返回）。
type InviteView struct {
	domainteam.Invite
	CreatedByName string
}

// InvitePreview 是 PreviewInvite 返回（不含 token）。
type InvitePreview struct {
	TeamID        string
	TeamName      string
	InviterName   string
	ExpiresAt     *time.Time
	AlreadyMember bool
}

// AcceptResult 是 AcceptInvite 返回。
type AcceptResult struct {
	TeamID        string
	AlreadyMember bool
}
