// Package team 是 team 用例服务层（单 domain 编排）。
//
// 接受 command，编排 domain/policy/repo 完成用例；禁止 import Hertz / hertz_gen / biz/dal。
// 事务：CreateTeam 需「插 team + 首任 admin」原子，由本 service 顶部 TxRunner.RunInTx 开启；
// 其余用例单语句，repo 自身 FromContext 透传 ctx 即可。
//
// 鉴权：每个用例内部 getMembership(teamID, userID) → 缺失 404 / role≠admin 403。
// 对齐 control-plane routes/teams.ts + routes/invites.ts。
package team

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	domainorganization "media_agent/media_auth/biz/domain/organization"
	domainteam "media_agent/media_auth/biz/domain/team"
	policyteam "media_agent/media_auth/biz/policy/team"
)

// Service 是 team 用例服务。
type Service struct {
	repo          domainteam.TeamRepo
	tx            TxRunner
	organizations OrganizationAccess
}

// OrganizationAccess 是 Team 用例依赖的企业事实源。Team 不复制企业成员关系。
type OrganizationAccess interface {
	Get(context.Context, string) (domainorganization.Organization, error)
	GetMembership(context.Context, string, string) (domainorganization.Membership, error)
	GetEntitlements(context.Context, string) ([]string, error)
}

// TxRunner 是事务抽象（dal.NewTxRunner 实现）。team service 仅 CreateTeam 用。
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// NewService 构造 team 用例服务。tx 可为 nil（仅 CreateTeam 不可用，其余用例不依赖事务）。
func NewService(repo domainteam.TeamRepo, tx TxRunner, organizations ...OrganizationAccess) *Service {
	s := &Service{repo: repo, tx: tx}
	if len(organizations) > 0 {
		s.organizations = organizations[0]
	}
	return s
}

// 用例错误（handler 据此映射 HTTP 状态）。
var (
	ErrInvalidName                    = errors.New("team name must not be empty")
	ErrInvalidRole                    = errors.New("invalid role: must be admin or member")
	ErrUserNotFound                   = errors.New("user not found")
	ErrTeamNotFound                   = domainteam.ErrTeamNotFound
	ErrInviteNotFound                 = domainteam.ErrInviteNotFound
	ErrNotMember                      = domainteam.ErrNotMember
	ErrForbidden                      = errors.New("admin only")
	ErrInviteExpired                  = errors.New("invite expired")
	ErrCannotDemoteLast               = policyteam.ErrCannotDemoteLastAdmin
	ErrCannotRemoveLast               = policyteam.ErrCannotRemoveLastAdmin
	ErrCreateTeamNoTx                 = errors.New("create team requires a transaction runner")
	ErrOrganizationRequired           = errors.New("organization_id is required")
	ErrOrganizationInactive           = errors.New("organization is not active")
	ErrOrganizationAdmin              = errors.New("organization admin only")
	ErrOrganizationMember             = errors.New("user must belong to the organization")
	ErrOrganizationNotFound           = domainorganization.ErrNotFound
	ErrInvalidEntitlementMode         = errors.New("entitlement_mode must be inherit or restricted")
	ErrEntitlementExceedsOrganization = errors.New("team entitlement exceeds organization entitlement")
	ErrInvalidPermission              = errors.New("invalid team permission")
)

// Authorize evaluates the authoritative Organization/Team facts. It is used
// for immediate fail-closed checks and as the reference decision for Keto
// shadow comparison.
func (s *Service) Authorize(ctx context.Context, userID, teamID, permission string) (AuthorizationView, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		return AuthorizationView{}, err
	}
	view := AuthorizationView{TeamID: team.ID, OrganizationID: team.OrganizationID, Permission: permission}
	if team.Status != "active" {
		return view, nil
	}
	view.TeamActive = true
	organization, err := s.organizations.Get(ctx, team.OrganizationID)
	if err != nil {
		return AuthorizationView{}, err
	}
	if organization.Status != "active" {
		return view, nil
	}
	view.OrganizationActive = true
	if membership, membershipErr := s.repo.GetMembership(ctx, teamID, userID); membershipErr == nil {
		view.TeamRole = string(membership.Role)
	} else if !errors.Is(membershipErr, domainteam.ErrNotMember) {
		return AuthorizationView{}, membershipErr
	}
	if membership, membershipErr := s.organizations.GetMembership(ctx, team.OrganizationID, userID); membershipErr == nil {
		view.OrganizationRole = membership.Role
	} else if !errors.Is(membershipErr, domainorganization.ErrNotMember) {
		return AuthorizationView{}, membershipErr
	}

	switch permission {
	case "view":
		view.Allowed = view.TeamRole != "" || view.OrganizationRole == "admin"
	case "manage":
		view.Allowed = view.TeamRole == "admin" || view.OrganizationRole == "admin"
	case "create_workspace", "use_materials", "use_workflow":
		if view.TeamRole == "" {
			return view, nil
		}
		configured, err := s.repo.GetEntitlements(ctx, teamID)
		if err != nil {
			return AuthorizationView{}, err
		}
		organizationKeys, err := s.organizations.GetEntitlements(ctx, team.OrganizationID)
		if err != nil {
			return AuthorizationView{}, err
		}
		view.EffectiveFeatureKeys = entitlementView(team.EntitlementMode, configured, organizationKeys).EffectiveFeatureKeys
		view.Allowed = hasFeature(view.EffectiveFeatureKeys, permissionFeaturePrefix(permission))
	default:
		return AuthorizationView{}, ErrInvalidPermission
	}
	return view, nil
}

// ListMyTeams 查当前 user 所属全部 team（含 my_role + member_count）。
func (s *Service) ListMyTeams(ctx context.Context, userID string) ([]TeamView, error) {
	summaries, err := s.repo.ListTeamsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]TeamView, 0, len(summaries))
	for _, sm := range summaries {
		out = append(out, TeamView{Team: sm.Team, MyRole: sm.MyRole, MemberCount: sm.MemberCount})
	}
	return out, nil
}

// CreateTeam 建团，创建者自动成为首任 admin（事务内插 team + admin 成员）。
func (s *Service) CreateTeam(ctx context.Context, userID string, cmd CreateTeamCommand) (TeamView, error) {
	if cmd.OrganizationID == "" {
		return TeamView{}, ErrOrganizationRequired
	}
	if cmd.Name == "" {
		return TeamView{}, ErrInvalidName
	}
	if s.tx == nil {
		return TeamView{}, ErrCreateTeamNoTx
	}
	if s.organizations == nil {
		return TeamView{}, ErrOrganizationRequired
	}
	organization, err := s.organizations.Get(ctx, cmd.OrganizationID)
	if err != nil {
		return TeamView{}, err
	}
	if organization.Status != "active" {
		return TeamView{}, ErrOrganizationInactive
	}
	membership, err := s.organizations.GetMembership(ctx, cmd.OrganizationID, userID)
	if err != nil || membership.Role != "admin" {
		return TeamView{}, ErrOrganizationAdmin
	}
	var team domainteam.Team
	err = s.tx.RunInTx(ctx, func(txCtx context.Context) error {
		t, err := s.repo.InsertTeam(txCtx, cmd.OrganizationID, userID, cmd.Name, cmd.Description)
		if err != nil {
			return err
		}
		if err := s.repo.AddMember(txCtx, t.ID, userID, domainteam.RoleAdmin); err != nil {
			return err
		}
		team = t
		return nil
	})
	if err != nil {
		return TeamView{}, err
	}
	return TeamView{Team: team, MyRole: domainteam.RoleAdmin, MemberCount: 1}, nil
}

// GetTeam 查 team 详情（仅成员可读；非成员 → 404 不暴露存在性）。
func (s *Service) GetTeam(ctx context.Context, userID, teamID string) (TeamView, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		return TeamView{}, err
	}
	membership, err := s.repo.GetMembership(ctx, teamID, userID)
	if err != nil {
		if !errors.Is(err, domainteam.ErrNotMember) {
			return TeamView{}, err
		}
		organizationMembership, orgErr := s.organizations.GetMembership(ctx, team.OrganizationID, userID)
		if orgErr != nil || organizationMembership.Role != "admin" {
			return TeamView{}, ErrTeamNotFound
		}
	}
	count, err := s.memberCount(ctx, teamID)
	if err != nil {
		return TeamView{}, err
	}
	return TeamView{Team: team, MyRole: membership.Role, MemberCount: count}, nil
}

func (s *Service) GetEntitlements(ctx context.Context, userID, teamID string) (EntitlementsView, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		return EntitlementsView{}, err
	}
	if _, err := s.repo.GetMembership(ctx, teamID, userID); err != nil {
		membership, orgErr := s.organizations.GetMembership(ctx, team.OrganizationID, userID)
		if orgErr != nil || membership.Role != "admin" {
			return EntitlementsView{}, ErrTeamNotFound
		}
	}
	configured, err := s.repo.GetEntitlements(ctx, teamID)
	if err != nil {
		return EntitlementsView{}, err
	}
	organizationKeys, err := s.organizations.GetEntitlements(ctx, team.OrganizationID)
	if err != nil {
		return EntitlementsView{}, err
	}
	return entitlementView(team.EntitlementMode, configured, organizationKeys), nil
}

func (s *Service) SetEntitlements(ctx context.Context, userID, teamID string, cmd SetEntitlementsCommand) (EntitlementsView, error) {
	if cmd.Mode != "inherit" && cmd.Mode != "restricted" {
		return EntitlementsView{}, ErrInvalidEntitlementMode
	}
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		return EntitlementsView{}, err
	}
	membership, err := s.organizations.GetMembership(ctx, team.OrganizationID, userID)
	if err != nil || membership.Role != "admin" {
		return EntitlementsView{}, ErrOrganizationAdmin
	}
	organizationKeys, err := s.organizations.GetEntitlements(ctx, team.OrganizationID)
	if err != nil {
		return EntitlementsView{}, err
	}
	requested := uniqueSorted(cmd.FeatureKeys)
	if cmd.Mode == "restricted" && !isSubset(requested, organizationKeys) {
		return EntitlementsView{}, ErrEntitlementExceedsOrganization
	}
	configured, err := s.repo.SetEntitlements(ctx, teamID, userID, cmd.Mode, requested)
	if err != nil {
		return EntitlementsView{}, err
	}
	return entitlementView(cmd.Mode, configured, organizationKeys), nil
}

// UpdateTeam 改 team name/description（admin only）。
func (s *Service) UpdateTeam(ctx context.Context, userID, teamID string, cmd UpdateTeamCommand) (TeamView, error) {
	if err := s.assertTeamOrOrganizationAdmin(ctx, teamID, userID); err != nil {
		return TeamView{}, err
	}
	team, err := s.repo.UpdateTeam(ctx, teamID, domainteam.TeamPatch{
		Name:        cmd.Name,
		Description: cmd.Description,
	})
	if err != nil {
		return TeamView{}, err
	}
	count, err := s.memberCount(ctx, teamID)
	if err != nil {
		return TeamView{}, err
	}
	return TeamView{Team: team, MyRole: domainteam.RoleAdmin, MemberCount: count}, nil
}

// DeleteTeam 删 team（admin only；级联清成员与邀请）。
func (s *Service) DeleteTeam(ctx context.Context, userID, teamID string) error {
	if err := s.assertOrganizationAdmin(ctx, teamID, userID); err != nil {
		return err
	}
	return s.repo.DeleteTeam(ctx, teamID)
}

// ListMembers 列 team 成员（仅成员可读）。
func (s *Service) ListMembers(ctx context.Context, userID, teamID string) ([]domainteam.MemberWithUser, error) {
	if _, err := s.assertMember(ctx, teamID, userID); err != nil {
		if !errors.Is(err, domainteam.ErrNotMember) {
			return nil, err
		}
		if err := s.assertOrganizationAdmin(ctx, teamID, userID); err != nil {
			return nil, ErrTeamNotFound
		}
	}
	return s.repo.ListMembers(ctx, teamID)
}

// AddMember 加成员（admin only；目标 user 必须存在；幂等）。
func (s *Service) AddMember(ctx context.Context, userID, teamID string, cmd AddMemberCommand) (domainteam.MemberWithUser, error) {
	if err := s.assertTeamOrOrganizationAdmin(ctx, teamID, userID); err != nil {
		return domainteam.MemberWithUser{}, err
	}
	role := domainteam.RoleMember
	if cmd.Role != nil {
		if !policyteam.IsValidRole(*cmd.Role) {
			return domainteam.MemberWithUser{}, ErrInvalidRole
		}
		role = domainteam.Role(*cmd.Role)
	}
	exists, err := s.repo.UserExists(ctx, cmd.UserID)
	if err != nil {
		return domainteam.MemberWithUser{}, err
	}
	if !exists {
		return domainteam.MemberWithUser{}, ErrUserNotFound
	}
	if s.organizations != nil {
		team, err := s.repo.GetTeam(ctx, teamID)
		if err != nil {
			return domainteam.MemberWithUser{}, err
		}
		if _, err := s.organizations.GetMembership(ctx, team.OrganizationID, cmd.UserID); err != nil {
			return domainteam.MemberWithUser{}, ErrOrganizationMember
		}
	}
	if err := s.repo.AddMember(ctx, teamID, cmd.UserID, role); err != nil {
		return domainteam.MemberWithUser{}, err
	}
	m, err := s.repo.GetMembership(ctx, teamID, cmd.UserID)
	if err != nil {
		return domainteam.MemberWithUser{}, err
	}
	name, err := s.repo.UserDisplayName(ctx, cmd.UserID)
	if err != nil {
		return domainteam.MemberWithUser{}, err
	}
	return domainteam.MemberWithUser{Membership: m, UserName: name}, nil
}

// UpdateMemberRole 改成员 role（admin only；末位 admin 保护）。
func (s *Service) UpdateMemberRole(ctx context.Context, userID, teamID, targetUserID, newRole string) error {
	if !policyteam.IsValidRole(newRole) {
		return ErrInvalidRole
	}
	if err := s.assertTeamOrOrganizationAdmin(ctx, teamID, userID); err != nil {
		return err
	}
	target, err := s.repo.GetMembership(ctx, teamID, targetUserID)
	if err != nil {
		return err // ErrNotMember → handler 404
	}
	adminCount, err := s.repo.CountAdmins(ctx, teamID)
	if err != nil {
		return err
	}
	if err := policyteam.CanDemoteMember(string(target.Role), newRole, adminCount); err != nil {
		return err
	}
	return s.repo.UpdateMemberRole(ctx, teamID, targetUserID, domainteam.Role(newRole))
}

// RemoveMember 移除成员（admin 可移任何人；非 admin 仅可 self-remove；末位 admin 保护）。
func (s *Service) RemoveMember(ctx context.Context, userID, teamID, targetUserID string) error {
	isSelf := targetUserID == userID
	if !isSelf {
		if err := s.assertTeamOrOrganizationAdmin(ctx, teamID, userID); err != nil {
			return err
		}
	} else {
		// self-remove 仍需是成员。
		if _, err := s.assertMember(ctx, teamID, userID); err != nil {
			return err
		}
	}
	target, err := s.repo.GetMembership(ctx, teamID, targetUserID)
	if err != nil {
		return err
	}
	adminCount, err := s.repo.CountAdmins(ctx, teamID)
	if err != nil {
		return err
	}
	if err := policyteam.CanRemoveMember(string(target.Role), adminCount); err != nil {
		return err
	}
	return s.repo.RemoveMember(ctx, teamID, targetUserID)
}

// ListInvites 列 team 未过期邀请（admin only）。
func (s *Service) ListInvites(ctx context.Context, userID, teamID string) ([]InviteView, error) {
	if err := s.assertAdmin(ctx, teamID, userID); err != nil {
		return nil, err
	}
	invites, err := s.repo.ListInvites(ctx, teamID)
	if err != nil {
		return nil, err
	}
	// 批量取创建者名（去重 user_id）。
	creatorIDs := make(map[string]struct{}, len(invites))
	for _, inv := range invites {
		creatorIDs[inv.CreatedBy] = struct{}{}
	}
	nameByID := make(map[string]string, len(creatorIDs))
	for uid := range creatorIDs {
		name, err := s.repo.UserDisplayName(ctx, uid)
		if err != nil {
			return nil, err
		}
		nameByID[uid] = name
	}
	out := make([]InviteView, 0, len(invites))
	for _, inv := range invites {
		out = append(out, InviteView{Invite: inv, CreatedByName: nameByID[inv.CreatedBy]})
	}
	return out, nil
}

// CreateInvite 签发邀请（admin only；默认 7 天过期；明文 token 仅本次返回）。
func (s *Service) CreateInvite(ctx context.Context, userID, teamID string, cmd CreateInviteCommand) (InviteView, error) {
	if err := s.assertAdmin(ctx, teamID, userID); err != nil {
		return InviteView{}, err
	}
	days := 7
	if cmd.ExpiresInDays != nil {
		days = *cmd.ExpiresInDays
	}
	var expiresAt *time.Time
	if days > 0 {
		t := time.Now().Add(time.Duration(days) * 24 * time.Hour)
		expiresAt = &t
	}
	// days <= 0 → nil（不过期）。对齐 control-plane：expires_in_days 仅在 >0 时设过期。
	inv, err := s.repo.CreateInvite(ctx, teamID, userID, expiresAt)
	if err != nil {
		return InviteView{}, err
	}
	name, err := s.repo.UserDisplayName(ctx, userID)
	if err != nil {
		return InviteView{}, err
	}
	return InviteView{Invite: inv, CreatedByName: name}, nil
}

// RevokeInvite 撤销邀请（admin only）。
func (s *Service) RevokeInvite(ctx context.Context, userID, teamID, token string) error {
	if err := s.assertAdmin(ctx, teamID, userID); err != nil {
		return err
	}
	return s.repo.DeleteInvite(ctx, token)
}

// PreviewInvite 预览邀请（任意已登录 user；token 不存在 404，过期 410，team 已删 404）。
// 同时返回调用方是否已是该 team 成员（already_member）。
func (s *Service) PreviewInvite(ctx context.Context, userID, token string) (InvitePreview, error) {
	inv, err := s.repo.GetInviteByToken(ctx, token)
	if err != nil {
		return InvitePreview{}, err
	}
	if policyteam.IsExpired(inv.ExpiresAt, time.Now()) {
		return InvitePreview{}, ErrInviteExpired
	}
	team, err := s.repo.GetTeam(ctx, inv.TeamID)
	if err != nil {
		return InvitePreview{}, err // team 已删 → ErrTeamNotFound（FK CASCADE 已清邀请，理论上不会到这）
	}
	inviterName, err := s.repo.UserDisplayName(ctx, inv.CreatedBy)
	if err != nil {
		return InvitePreview{}, err
	}
	// 判定调用方是否已是成员（GetMembership 返回 ErrNotMember 即非成员）。
	alreadyMember := true
	if _, err := s.repo.GetMembership(ctx, team.ID, userID); err != nil {
		if !errors.Is(err, domainteam.ErrNotMember) {
			return InvitePreview{}, err
		}
		alreadyMember = false
	}
	return InvitePreview{
		TeamID:        team.ID,
		TeamName:      team.Name,
		InviterName:   inviterName,
		ExpiresAt:     inv.ExpiresAt,
		AlreadyMember: alreadyMember,
	}, nil
}

// AcceptInvite 接受邀请入团（任意已登录 user；已入团幂等返 already_member=true）。
func (s *Service) AcceptInvite(ctx context.Context, userID, token string) (AcceptResult, error) {
	inv, err := s.repo.GetInviteByToken(ctx, token)
	if err != nil {
		return AcceptResult{}, err
	}
	if policyteam.IsExpired(inv.ExpiresAt, time.Now()) {
		return AcceptResult{}, ErrInviteExpired
	}
	team, err := s.repo.GetTeam(ctx, inv.TeamID)
	if err != nil {
		return AcceptResult{}, err
	}
	if s.organizations != nil {
		if _, err := s.organizations.GetMembership(ctx, team.OrganizationID, userID); err != nil {
			return AcceptResult{}, ErrOrganizationMember
		}
	}
	// 已是成员 → 幂等返回。
	if _, err := s.repo.GetMembership(ctx, team.ID, userID); err == nil {
		return AcceptResult{TeamID: team.ID, AlreadyMember: true}, nil
	} else if !errors.Is(err, domainteam.ErrNotMember) {
		return AcceptResult{}, err
	}
	if err := s.repo.AddMember(ctx, team.ID, userID, domainteam.RoleMember); err != nil {
		return AcceptResult{}, err
	}
	return AcceptResult{TeamID: team.ID, AlreadyMember: false}, nil
}

// ── helpers ──────────────────────────────────────────────────────────

// assertMember 校验 userID 是 team 成员，返回 membership；否则返回 ErrNotMember（handler 映射 404）。
func (s *Service) assertMember(ctx context.Context, teamID, userID string) (domainteam.Membership, error) {
	m, err := s.repo.GetMembership(ctx, teamID, userID)
	if err != nil {
		return domainteam.Membership{}, err
	}
	return m, nil
}

// assertAdmin 校验 userID 是 team admin；非成员 → ErrNotMember，非 admin → ErrForbidden。
func (s *Service) assertAdmin(ctx context.Context, teamID, userID string) error {
	m, err := s.assertMember(ctx, teamID, userID)
	if err != nil {
		return err
	}
	if m.Role != domainteam.RoleAdmin {
		return ErrForbidden
	}
	return nil
}

func (s *Service) assertTeamOrOrganizationAdmin(ctx context.Context, teamID, userID string) error {
	if err := s.assertAdmin(ctx, teamID, userID); err == nil {
		return nil
	} else if !errors.Is(err, domainteam.ErrNotMember) && !errors.Is(err, ErrForbidden) {
		return err
	}
	return s.assertOrganizationAdmin(ctx, teamID, userID)
}

func (s *Service) assertOrganizationAdmin(ctx context.Context, teamID, userID string) error {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		return err
	}
	membership, err := s.organizations.GetMembership(ctx, team.OrganizationID, userID)
	if err != nil {
		return ErrNotMember
	}
	if membership.Role != "admin" {
		return ErrOrganizationAdmin
	}
	return nil
}

// memberCount 取 team 成员数（GetTeam/CreateTeam 响应填充用）。
func (s *Service) memberCount(ctx context.Context, teamID string) (int, error) {
	members, err := s.repo.ListMembers(ctx, teamID)
	if err != nil {
		return 0, err
	}
	return len(members), nil
}

func entitlementView(mode string, configured, organizationKeys []string) EntitlementsView {
	configured = uniqueSorted(configured)
	organizationKeys = uniqueSorted(organizationKeys)
	effective := organizationKeys
	if mode == "restricted" {
		effective = intersection(organizationKeys, configured)
	}
	return EntitlementsView{Mode: mode, ConfiguredFeatureKeys: configured, EffectiveFeatureKeys: effective}
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isSubset(values, allowed []string) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := allowedSet[value]; !ok {
			return false
		}
	}
	return true
}

func intersection(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	out := make([]string, 0)
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func permissionFeaturePrefix(permission string) string {
	switch permission {
	case "create_workspace":
		return "workspace"
	case "use_materials":
		return "materials"
	case "use_workflow":
		return "workflow"
	default:
		return ""
	}
}

func hasFeature(keys []string, prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, key := range keys {
		if key == prefix || strings.HasPrefix(key, prefix+".") {
			return true
		}
	}
	return false
}
