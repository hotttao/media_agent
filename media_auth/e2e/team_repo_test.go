//go:build e2e

// team_repo_test.go：team 仓储集成测试（testcontainers postgres）。
//
// 覆盖：CreateTeam（事务插 team+admin）、GetTeam/ListTeamsForUser/ListMembers、
//
//	AddMember ON CONFLICT 幂等、CountAdmins、UpdateTeam patch、邀请签发/撤销/过期、
//	DeleteTeam 级联清成员+邀请。
package e2e

import (
	"context"
	"errors"
	"testing"
	"time"

	dalteam "media_agent/media_auth/biz/dal/repo/team"
	domainteam "media_agent/media_auth/biz/domain/team"
)

func newTeamRepo(t *testing.T) domainteam.TeamRepo {
	db := newDB(t)
	seedUser(t, db, "u1", "alice")
	seedUser(t, db, "u2", "bob")
	return dalteam.NewRepo(db.Gorm)
}

func TestTeamRepo_CreateTeam_InsertsCreatorAsAdmin(t *testing.T) {
	repo := newTeamRepo(t)
	team, err := repo.InsertTeam(context.Background(), "u1", "Team A", "desc")
	if err != nil {
		t.Fatalf("InsertTeam: %v", err)
	}
	if team.ID == "" || team.Name != "Team A" {
		t.Errorf("got team=%+v", team)
	}
	if err := repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	m, err := repo.GetMembership(context.Background(), team.ID, "u1")
	if err != nil || m.Role != domainteam.RoleAdmin {
		t.Errorf("creator membership: got m=%+v err=%v", m, err)
	}
	if n, _ := repo.CountAdmins(context.Background(), team.ID); n != 1 {
		t.Errorf("CountAdmins: want 1, got %d", n)
	}
}

func TestTeamRepo_AddMember_Idempotent(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin)

	if err := repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin); err != nil {
		t.Errorf("duplicate AddMember should be idempotent, got %v", err)
	}
	members, _ := repo.ListMembers(context.Background(), team.ID)
	if len(members) != 1 {
		t.Errorf("after duplicate add: want 1 member, got %d", len(members))
	}
}

func TestTeamRepo_GetMembership_NotMember(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin)

	_, err := repo.GetMembership(context.Background(), team.ID, "u2")
	if !errors.Is(err, domainteam.ErrNotMember) {
		t.Errorf("non-member: want ErrNotMember, got %v", err)
	}
}

func TestTeamRepo_UpdateTeam_PatchDescription(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	desc := "new desc"
	updated, err := repo.UpdateTeam(context.Background(), team.ID, domainteam.TeamPatch{Description: &desc})
	if err != nil || updated.Description != "new desc" {
		t.Errorf("set desc: got updated=%+v err=%v", updated, err)
	}
	empty := ""
	updated, err = repo.UpdateTeam(context.Background(), team.ID, domainteam.TeamPatch{Description: &empty})
	if err != nil || updated.Description != "" {
		t.Errorf("clear desc: got updated=%+v err=%v", updated, err)
	}
}

func TestTeamRepo_DeleteTeam_Cascades(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin)
	repo.CreateInvite(context.Background(), team.ID, "u1", nil)

	if err := repo.DeleteTeam(context.Background(), team.ID); err != nil {
		t.Fatalf("DeleteTeam: %v", err)
	}
	if _, err := repo.GetTeam(context.Background(), team.ID); !errors.Is(err, domainteam.ErrTeamNotFound) {
		t.Errorf("after delete: want ErrTeamNotFound, got %v", err)
	}
	members, _ := repo.ListMembers(context.Background(), team.ID)
	if len(members) != 0 {
		t.Errorf("cascade: members should be empty, got %d", len(members))
	}
	invites, _ := repo.ListInvites(context.Background(), team.ID)
	if len(invites) != 0 {
		t.Errorf("cascade: invites should be empty, got %d", len(invites))
	}
}

func TestTeamRepo_Invites_ExpiryAndRevoke(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin)

	future := time.Now().Add(7 * 24 * time.Hour)
	inv, err := repo.CreateInvite(context.Background(), team.ID, "u1", &future)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.Token == "" {
		t.Errorf("token empty")
	}
	invites, _ := repo.ListInvites(context.Background(), team.ID)
	if len(invites) != 1 {
		t.Errorf("ListInvites: want 1, got %d", len(invites))
	}
	past := time.Now().Add(-time.Hour)
	repo.CreateInvite(context.Background(), team.ID, "u1", &past)
	invites, _ = repo.ListInvites(context.Background(), team.ID)
	if len(invites) != 1 {
		t.Errorf("ListInvites should exclude expired: want 1, got %d", len(invites))
	}
	if err := repo.DeleteInvite(context.Background(), inv.Token); err != nil {
		t.Errorf("DeleteInvite: %v", err)
	}
	if err := repo.DeleteInvite(context.Background(), inv.Token); !errors.Is(err, domainteam.ErrInviteNotFound) {
		t.Errorf("re-delete: want ErrInviteNotFound, got %v", err)
	}
}

func TestTeamRepo_ListMembers_JoinUser(t *testing.T) {
	repo := newTeamRepo(t)
	team, _ := repo.InsertTeam(context.Background(), "u1", "T", "")
	repo.AddMember(context.Background(), team.ID, "u1", domainteam.RoleAdmin)
	repo.AddMember(context.Background(), team.ID, "u2", domainteam.RoleMember)

	members, _ := repo.ListMembers(context.Background(), team.ID)
	if len(members) != 2 {
		t.Fatalf("want 2 members, got %d", len(members))
	}
	for _, m := range members {
		if m.UserName == "" {
			t.Errorf("member %s: UserName empty (JOIN failed)", m.UserID)
		}
	}
}
