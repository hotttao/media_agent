package virtual_ip

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domain "media_agent/media_materials/biz/domain/virtual_ip"
	"media_agent/media_materials/biz/shared"
)

// Service virtual_ip 用例编排。持有 Repo + GrantRepo。
// visibility ACL：private=owner；public=任意；team 暂按 owner（team 成员来源待 media_auth 接入）。
// Grant/Revoke/ChangeVisibility 仅 owner 可执行。
type Service struct {
	repo         domain.Repo
	grants       domain.GrantRepo
	bindingEnder shared.BindingEnder // 注入：owner 软删时失效 material_bindings
}

func NewService(repo domain.Repo, grants domain.GrantRepo) *Service {
	return &Service{repo: repo, grants: grants}
}

// SetBindingEnder 注入 material BindingEnder（owner 软删时失效其 material_bindings）。
func (s *Service) SetBindingEnder(ender shared.BindingEnder) { s.bindingEnder = ender }

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.VirtualIp, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, errors.New("virtual_ip name is required")
	}
	if !shared.IsKnownScope(cmd.Scope) {
		return nil, errors.New("virtual_ip scope is not supported")
	}
	vis := domain.Visibility(cmd.Visibility)
	if vis == "" {
		vis = domain.VisibilityPrivate
	}
	if err := validVisibility(vis); err != nil {
		return nil, err
	}
	v := &domain.VirtualIp{
		ID: "vip_" + uuid.NewString(), Scope: cmd.Scope, Name: name, Description: cmd.Description,
		AvatarURL: cmd.AvatarURL, FullBodyURL: cmd.FullBodyURL,
		ThreeViewURL: cmd.ThreeViewURL, NineViewURL: cmd.NineViewURL,
		Status: domain.StatusActive, Visibility: vis, OwnerUserID: cmd.OwnerUserID,
	}
	if err := s.repo.Save(ctx, v); err != nil {
		return nil, fmt.Errorf("save virtual_ip: %w", err)
	}
	return s.repo.Get(ctx, v.ID)
}

func (s *Service) Get(ctx context.Context, id, callerID string, callerTeams []string) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("virtual_ip %q: %w", id, err)
	}
	if !s.canRead(ctx, v, callerID, callerTeams) {
		return nil, fmt.Errorf("virtual_ip %q not accessible", id)
	}
	return v, nil
}

func (s *Service) List(ctx context.Context, q ListQuery, callerID string, callerTeams []string) ([]*domain.VirtualIp, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	out := items[:0]
	for _, v := range items {
		if s.canRead(ctx, v, callerID, callerTeams) {
			out = append(out, v)
		}
	}
	return out, total, nil
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != cmd.CallerID {
		return nil, errors.New("only owner can update virtual_ip")
	}
	if cmd.Name != "" {
		v.Name = cmd.Name
	}
	v.Description = cmd.Description
	v.AvatarURL = cmd.AvatarURL
	v.FullBodyURL = cmd.FullBodyURL
	v.ThreeViewURL = cmd.ThreeViewURL
	v.NineViewURL = cmd.NineViewURL
	if err := s.repo.Update(ctx, v); err != nil {
		return nil, fmt.Errorf("update virtual_ip: %w", err)
	}
	return s.repo.Get(ctx, cmd.ID)
}

func (s *Service) Archive(ctx context.Context, id string) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	v.Status = domain.StatusArchived
	if err := s.repo.Update(ctx, v); err != nil {
		return nil, fmt.Errorf("archive virtual_ip: %w", err)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, callerID string) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != callerID {
		return nil, errors.New("only owner can delete virtual_ip")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete virtual_ip: %w", err)
	}
	if s.bindingEnder != nil {
		if err := s.bindingEnder.EndBindingsForOwner(ctx, "virtual_ip", id); err != nil {
			return nil, fmt.Errorf("end bindings for virtual_ip: %w", err)
		}
	}
	return v, nil
}

// Grant owner 给 team 授权。仅 owner。
func (s *Service) Grant(ctx context.Context, cmd GrantCommand) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != cmd.GrantedBy {
		return nil, errors.New("only owner can grant virtual_ip")
	}
	perm := domain.Permission(cmd.Permission)
	if perm != domain.PermissionViewer && perm != domain.PermissionEditor {
		return nil, fmt.Errorf("invalid permission %q", cmd.Permission)
	}
	g := &domain.Grant{
		VirtualIPID: cmd.ID, TeamID: cmd.TeamID, Permission: perm, GrantedBy: cmd.GrantedBy,
	}
	if err := s.grants.AddGrant(ctx, g); err != nil {
		return nil, fmt.Errorf("add grant: %w", err)
	}
	return s.repo.Get(ctx, cmd.ID)
}

// Revoke owner 撤权。仅 owner。
func (s *Service) Revoke(ctx context.Context, id, teamID, callerID string) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != callerID {
		return nil, errors.New("only owner can revoke virtual_ip grant")
	}
	if err := s.grants.RemoveGrant(ctx, id, teamID); err != nil {
		return nil, fmt.Errorf("remove grant: %w", err)
	}
	return v, nil
}

// ChangeVisibility 改可见性。仅 owner。
func (s *Service) ChangeVisibility(ctx context.Context, id, visibility, callerID string) (*domain.VirtualIp, error) {
	v, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if v.OwnerUserID != callerID {
		return nil, errors.New("only owner can change visibility")
	}
	vis := domain.Visibility(visibility)
	if err := validVisibility(vis); err != nil {
		return nil, err
	}
	v.Visibility = vis
	if err := s.repo.Update(ctx, v); err != nil {
		return nil, fmt.Errorf("update visibility: %w", err)
	}
	return s.repo.Get(ctx, id)
}

// —— helpers ——

func (s *Service) canRead(ctx context.Context, v *domain.VirtualIp, callerID string, callerTeams []string) bool {
	if v.OwnerUserID == callerID {
		return true
	}
	if v.Visibility == domain.VisibilityPublic {
		return true
	}
	if v.Visibility == domain.VisibilityTeam && len(callerTeams) > 0 {
		// 查 grants：caller 的某个 team 有 grant on this vip
		grants, err := s.grants.ListGrants(ctx, v.ID)
		if err == nil {
			grantTeams := make(map[string]bool, len(grants))
			for _, g := range grants {
				grantTeams[g.TeamID] = true
			}
			for _, t := range callerTeams {
				if grantTeams[t] {
					return true
				}
			}
		}
	}
	return false
}

func validVisibility(v domain.Visibility) error {
	switch v {
	case domain.VisibilityPrivate, domain.VisibilityTeam, domain.VisibilityPublic:
		return nil
	}
	return fmt.Errorf("invalid visibility %q", v)
}

// GetOwnerScope 实现 material.OwnerScopeLookup 端口。
func (s *Service) GetOwnerScope(ctx context.Context, ownerKind, ownerID string) (string, error) {
	if ownerKind != "virtual_ip" {
		return "", fmt.Errorf("owner_kind %q not handled by virtual_ip service", ownerKind)
	}
	v, err := s.repo.Get(ctx, ownerID)
	if err != nil {
		return "", err
	}
	return v.Scope, nil
}
