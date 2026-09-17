// service_test.go：team 用例 service 单测（fake repo + fake tx，不依赖 DB/网络）。
//
// 覆盖：CreateTeam（事务插 team+admin）、UpdateMemberRole（末位 admin 保护）、
//
//	RemoveMember（self-remove / 末位 admin 保护）、AcceptInvite（幂等 / 新加入）、
//	AddMember（目标 user 不存在 → ErrUserNotFound）、ListMyTeams。
package team

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainorganization "media_agent/media_auth/biz/domain/organization"
	domainteam "media_agent/media_auth/biz/domain/team"
)

// ── fakes ────────────────────────────────────────────────────────────

// fakeRepo 实现 domainteam.TeamRepo（内存版）。
type fakeRepo struct {
	mu      sync.Mutex
	teams   map[string]domainteam.Team
	members map[string]map[string]domainteam.Membership // teamID → userID → membership
	invites map[string]domainteam.Invite                // token → invite
	users   map[string]bool                             // 存在的 user id
	nextID  int
}

func newFakeRepo(existingUsers ...string) *fakeRepo {
	r := &fakeRepo{
		teams:   map[string]domainteam.Team{},
		members: map[string]map[string]domainteam.Membership{},
		invites: map[string]domainteam.Invite{},
		users:   map[string]bool{},
	}
	for _, u := range existingUsers {
		r.users[u] = true
	}
	return r
}

func (r *fakeRepo) InsertTeam(_ context.Context, organizationID, createdBy, name, description string) (domainteam.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	id := "t" + itoa(r.nextID)
	t := domainteam.Team{ID: id, OrganizationID: organizationID, Name: name, Description: description, EntitlementMode: "inherit", Status: "active", CreatedBy: createdBy, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	r.teams[id] = t
	r.members[id] = map[string]domainteam.Membership{}
	return t, nil
}

type allowOrg struct{}

func (allowOrg) Get(context.Context, string) (domainorganization.Organization, error) {
	return domainorganization.Organization{ID: "org1", Status: "active"}, nil
}
func (allowOrg) GetMembership(_ context.Context, organizationID, userID string) (domainorganization.Membership, error) {
	return domainorganization.Membership{OrganizationID: organizationID, UserID: userID, Role: "admin"}, nil
}

func newTestService(repo *fakeRepo, tx TxRunner) *Service {
	return NewService(repo, tx, allowOrg{})
}

func (r *fakeRepo) GetTeam(_ context.Context, id string) (domainteam.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return domainteam.Team{}, domainteam.ErrTeamNotFound
	}
	return t, nil
}

func (r *fakeRepo) UpdateTeam(_ context.Context, id string, patch domainteam.TeamPatch) (domainteam.Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.teams[id]
	if !ok {
		return domainteam.Team{}, domainteam.ErrTeamNotFound
	}
	if patch.Name != nil {
		t.Name = *patch.Name
	}
	if patch.Description != nil {
		t.Description = *patch.Description
	}
	t.UpdatedAt = time.Now()
	r.teams[id] = t
	return t, nil
}

func (r *fakeRepo) DeleteTeam(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.teams[id]; !ok {
		return domainteam.ErrTeamNotFound
	}
	delete(r.teams, id)
	delete(r.members, id)
	return nil
}

func (r *fakeRepo) ListTeamsForUser(_ context.Context, userID string) ([]domainteam.TeamSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domainteam.TeamSummary
	for tid, members := range r.members {
		if m, ok := members[userID]; ok {
			out = append(out, domainteam.TeamSummary{Team: r.teams[tid], MyRole: m.Role, MemberCount: len(members)})
		}
	}
	return out, nil
}

func (r *fakeRepo) GetMembership(_ context.Context, teamID, userID string) (domainteam.Membership, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[teamID][userID]
	if !ok {
		return domainteam.Membership{}, domainteam.ErrNotMember
	}
	return m, nil
}

func (r *fakeRepo) ListMembers(_ context.Context, teamID string) ([]domainteam.MemberWithUser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domainteam.MemberWithUser
	for uid, m := range r.members[teamID] {
		out = append(out, domainteam.MemberWithUser{Membership: m, UserName: "name-" + uid})
	}
	return out, nil
}

func (r *fakeRepo) CountAdmins(_ context.Context, teamID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, m := range r.members[teamID] {
		if m.Role == domainteam.RoleAdmin {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) AddMember(_ context.Context, teamID, userID string, role domainteam.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members[teamID] == nil {
		r.members[teamID] = map[string]domainteam.Membership{}
	}
	if _, exists := r.members[teamID][userID]; exists {
		return nil // ON CONFLICT DO NOTHING
	}
	r.members[teamID][userID] = domainteam.Membership{TeamID: teamID, UserID: userID, Role: role, JoinedAt: time.Now()}
	return nil
}

func (r *fakeRepo) UpdateMemberRole(_ context.Context, teamID, userID string, role domainteam.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[teamID][userID]
	if !ok {
		return domainteam.ErrMemberNotFound
	}
	m.Role = role
	r.members[teamID][userID] = m
	return nil
}

func (r *fakeRepo) RemoveMember(_ context.Context, teamID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[teamID][userID]; !ok {
		return domainteam.ErrMemberNotFound
	}
	delete(r.members[teamID], userID)
	return nil
}

func (r *fakeRepo) UserExists(_ context.Context, userID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.users[userID], nil
}

func (r *fakeRepo) UserDisplayName(_ context.Context, userID string) (string, error) {
	return "name-" + userID, nil
}

func (r *fakeRepo) CreateInvite(_ context.Context, teamID, createdBy string, expiresAt *time.Time) (domainteam.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	tok := "tinv_" + itoa(r.nextID)
	inv := domainteam.Invite{Token: tok, TeamID: teamID, CreatedBy: createdBy, ExpiresAt: expiresAt, CreatedAt: time.Now()}
	r.invites[tok] = inv
	return inv, nil
}

func (r *fakeRepo) GetInviteByToken(_ context.Context, token string) (domainteam.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inv, ok := r.invites[token]
	if !ok {
		return domainteam.Invite{}, domainteam.ErrInviteNotFound
	}
	return inv, nil
}

func (r *fakeRepo) ListInvites(_ context.Context, teamID string) ([]domainteam.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domainteam.Invite
	for _, inv := range r.invites {
		if inv.TeamID == teamID {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (r *fakeRepo) DeleteInvite(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.invites[token]; !ok {
		return domainteam.ErrInviteNotFound
	}
	delete(r.invites, token)
	return nil
}

// fakeTx 实现 service.TxRunner（直接执行 fn，不真开事务）。
type fakeTx struct{}

func (fakeTx) RunInTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

// noTx 是 nil TxRunner（CreateTeam 应报错）。
type noTx struct{}

func (noTx) RunInTx(context.Context, func(context.Context) error) error {
	panic("should not be called")
}

// itoa 简易整数转字符串（避免 import strconv 增加噪声）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ── 用例 ──────────────────────────────────────────────────────────────

func TestCreateTeam(t *testing.T) {
	repo := newFakeRepo("u1")
	svc := newTestService(repo, fakeTx{})

	view, err := svc.CreateTeam(context.Background(), "u1", CreateTeamCommand{OrganizationID: "org1", Name: "Team A", Description: "desc"})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if view.Name != "Team A" || view.MyRole != domainteam.RoleAdmin || view.MemberCount != 1 {
		t.Errorf("got view=%+v", view)
	}
	// 创建者应是成员 + admin。
	m, err := repo.GetMembership(context.Background(), view.ID, "u1")
	if err != nil || m.Role != domainteam.RoleAdmin {
		t.Errorf("creator should be admin member, got m=%+v err=%v", m, err)
	}
}

func TestCreateTeam_EmptyName(t *testing.T) {
	svc := newTestService(newFakeRepo("u1"), fakeTx{})
	_, err := svc.CreateTeam(context.Background(), "u1", CreateTeamCommand{OrganizationID: "org1", Name: ""})
	if !errors.Is(err, ErrInvalidName) {
		t.Errorf("want ErrInvalidName, got %v", err)
	}
}

func TestCreateTeam_NoTxRunner(t *testing.T) {
	svc := newTestService(newFakeRepo("u1"), nil)
	_, err := svc.CreateTeam(context.Background(), "u1", CreateTeamCommand{OrganizationID: "org1", Name: "x"})
	if !errors.Is(err, ErrCreateTeamNoTx) {
		t.Errorf("want ErrCreateTeamNoTx, got %v", err)
	}
}

// seedTeamWithAdmin 建 team + 1 admin（创建者），返回 team id。
func seedTeamWithAdmin(t *testing.T, svc *Service, repo *fakeRepo, creator string) string {
	t.Helper()
	view, err := svc.CreateTeam(context.Background(), creator, CreateTeamCommand{OrganizationID: "org1", Name: "T"})
	if err != nil {
		t.Fatalf("seed CreateTeam: %v", err)
	}
	return view.ID
}

func TestUpdateMemberRole_LastAdminProtection(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")
	// 加 u2 为 member。
	if _, err := svc.AddMember(context.Background(), "u1", teamID, AddMemberCommand{UserID: "u2"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// 末位 admin 降级自己 → 409（u1 是唯一 admin）。
	err := svc.UpdateMemberRole(context.Background(), "u1", teamID, "u1", "member")
	if !errors.Is(err, ErrCannotDemoteLast) {
		t.Errorf("demote last admin: want ErrCannotDemoteLast, got %v", err)
	}

	// 把 u2 升 admin 后，u1 可降级自己。
	if err := svc.UpdateMemberRole(context.Background(), "u1", teamID, "u2", "admin"); err != nil {
		t.Fatalf("promote u2: %v", err)
	}
	if err := svc.UpdateMemberRole(context.Background(), "u1", teamID, "u1", "member"); err != nil {
		t.Errorf("demote u1 after second admin: %v", err)
	}
}

func TestRemoveMember_SelfAndLastAdmin(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")

	// u2 加入。
	svc.AddMember(context.Background(), "u1", teamID, AddMemberCommand{UserID: "u2"})

	// u2 self-remove → 允许（非 admin）。
	if err := svc.RemoveMember(context.Background(), "u2", teamID, "u2"); err != nil {
		t.Errorf("self-remove: %v", err)
	}

	// u1（末位 admin）self-remove → 409。
	err := svc.RemoveMember(context.Background(), "u1", teamID, "u1")
	if !errors.Is(err, ErrCannotRemoveLast) {
		t.Errorf("remove last admin: want ErrCannotRemoveLast, got %v", err)
	}
}

func TestRemoveMember_AdminCanRemoveOthers(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")
	svc.AddMember(context.Background(), "u1", teamID, AddMemberCommand{UserID: "u2"})

	// 非 admin 移除他人 → 403。
	err := svc.RemoveMember(context.Background(), "u2", teamID, "u1")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("non-admin remove others: want ErrForbidden, got %v", err)
	}

	// admin 移除他人 → 允许。
	if err := svc.RemoveMember(context.Background(), "u1", teamID, "u2"); err != nil {
		t.Errorf("admin remove others: %v", err)
	}
}

func TestAcceptInvite_Idempotent(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")

	// u1 创建邀请。
	inv, err := svc.CreateInvite(context.Background(), "u1", teamID, CreateInviteCommand{})
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// u2 首次接受 → already_member=false。
	res, err := svc.AcceptInvite(context.Background(), "u2", inv.Token)
	if err != nil || res.AlreadyMember {
		t.Errorf("first accept: got res=%+v err=%v", res, err)
	}
	// u2 再次接受 → already_member=true（幂等）。
	res, err = svc.AcceptInvite(context.Background(), "u2", inv.Token)
	if err != nil || !res.AlreadyMember {
		t.Errorf("second accept: got res=%+v err=%v", res, err)
	}
}

func TestAcceptInvite_Expired(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")
	// 手动塞过期邀请。
	past := time.Now().Add(-time.Hour)
	inv, _ := repo.CreateInvite(context.Background(), teamID, "u1", &past)

	_, err := svc.AcceptInvite(context.Background(), "u2", inv.Token)
	if !errors.Is(err, ErrInviteExpired) {
		t.Errorf("expired invite: want ErrInviteExpired, got %v", err)
	}
}

func TestAddMember_UserNotFound(t *testing.T) {
	repo := newFakeRepo("u1") // u2 不存在
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")

	_, err := svc.AddMember(context.Background(), "u1", teamID, AddMemberCommand{UserID: "u2"})
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("add nonexistent user: want ErrUserNotFound, got %v", err)
	}
}

func TestAddMember_InvalidRole(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")
	role := "owner"
	_, err := svc.AddMember(context.Background(), "u1", teamID, AddMemberCommand{UserID: "u2", Role: &role})
	if !errors.Is(err, ErrInvalidRole) {
		t.Errorf("invalid role: want ErrInvalidRole, got %v", err)
	}
}

func TestGetTeam_NonMemberNotFound(t *testing.T) {
	repo := newFakeRepo("u1", "u2")
	svc := newTestService(repo, fakeTx{})
	teamID := seedTeamWithAdmin(t, svc, repo, "u1")

	// u2 非成员 → 404（不暴露存在性）。
	_, err := svc.GetTeam(context.Background(), "u2", teamID)
	if !errors.Is(err, ErrNotMember) {
		t.Errorf("non-member get: want ErrNotMember, got %v", err)
	}
}

func TestListMyTeams(t *testing.T) {
	repo := newFakeRepo("u1")
	svc := newTestService(repo, fakeTx{})
	seedTeamWithAdmin(t, svc, repo, "u1")
	seedTeamWithAdmin(t, svc, repo, "u1")

	teams, err := svc.ListMyTeams(context.Background(), "u1")
	if err != nil {
		t.Fatalf("ListMyTeams: %v", err)
	}
	if len(teams) != 2 {
		t.Errorf("want 2 teams, got %d", len(teams))
	}
	for _, tm := range teams {
		if tm.MyRole != domainteam.RoleAdmin || tm.MemberCount != 1 {
			t.Errorf("unexpected team summary: %+v", tm)
		}
	}
}
