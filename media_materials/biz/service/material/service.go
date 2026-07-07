package material

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domain "media_agent/media_materials/biz/domain/material"
)

// Service material 用例编排。持有 Repo + BindingRepo + AssetKindLookup + OwnerScopeLookup。
type Service struct {
	repo       domain.Repo
	bindings   domain.BindingRepo
	kindLookup domain.AssetKindLookup
	ownerScope domain.OwnerScopeLookup
}

func NewService(repo domain.Repo, bindings domain.BindingRepo, lookup domain.AssetKindLookup, ownerScope domain.OwnerScopeLookup) *Service {
	return &Service{repo: repo, bindings: bindings, kindLookup: lookup, ownerScope: ownerScope}
}

// Create 创建 material。
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Material, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, errors.New("material name is required")
	}
	if strings.TrimSpace(cmd.URL) == "" && strings.TrimSpace(cmd.Content) == "" {
		return nil, errors.New("material url or content is required")
	}
	// asset_kind 校验：已注册、resource_type=MATERIAL、未停用
	rt, _, status, err := s.kindLookup.Lookup(ctx, cmd.AssetKind)
	if err != nil {
		return nil, fmt.Errorf("asset_kind %q: %w", cmd.AssetKind, err)
	}
	if rt != "MATERIAL" {
		return nil, fmt.Errorf("asset_kind %q resource_type=%q, want MATERIAL", cmd.AssetKind, rt)
	}
	if status != "SYSTEM" && status != "ENABLED" {
		return nil, fmt.Errorf("asset_kind %q is %s, cannot reference", cmd.AssetKind, status)
	}
	vis := domain.Visibility(cmd.Visibility)
	if vis == "" {
		vis = domain.VisibilityPrivate
	}
	if err := validVisibility(vis); err != nil {
		return nil, err
	}
	m := &domain.Material{
		ID:          "mat_" + uuid.NewString(),
		Scope:       cmd.Scope,
		Name:        name,
		AssetKind:   cmd.AssetKind,
		URL:         cmd.URL,
		Content:     cmd.Content,
		OwnerUserID: cmd.OwnerUserID,
		Visibility:  vis,
		TeamID:      cmd.TeamID,
	}
	if err := s.repo.Save(ctx, m); err != nil {
		return nil, fmt.Errorf("save material: %w", err)
	}
	return s.repo.Get(ctx, m.ID)
}

// Get 取（含可见性校验：caller 非 owner 且非 public → 拒）。
func (s *Service) Get(ctx context.Context, id, callerID string, callerTeams []string) (*domain.Material, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.canRead(m, callerID, callerTeams) {
		return nil, fmt.Errorf("material %q not accessible", id)
	}
	return m, nil
}

// List 分页（仅返回 caller 可见的：自己的 / public；team 待接入）。
func (s *Service) List(ctx context.Context, q ListQuery, callerID string, callerTeams []string) ([]*domain.Material, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	out := items[:0]
	for _, m := range items {
		if s.canRead(m, callerID, callerTeams) {
			out = append(out, m)
		}
	}
	return out, total, nil
}

// Update 改可变字段。仅 owner。
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Material, error) {
	m, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if m.OwnerUserID != cmd.CallerID {
		return nil, errors.New("only owner can update material")
	}
	if cmd.Name != "" {
		m.Name = cmd.Name
	}
	m.URL = cmd.URL
	m.Content = cmd.Content
	if cmd.Visibility != "" {
		vis := domain.Visibility(cmd.Visibility)
		if err := validVisibility(vis); err != nil {
			return nil, err
		}
		m.Visibility = vis
	}
	m.TeamID = cmd.TeamID
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("update material: %w", err)
	}
	return s.repo.Get(ctx, cmd.ID)
}

// Delete 软删。仅 owner。
func (s *Service) Delete(ctx context.Context, id, callerID string) (*domain.Material, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.OwnerUserID != callerID {
		return nil, errors.New("only owner can delete material")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete material: %w", err)
	}
	return m, nil
}

// AddBinding material ↔ owner。同 scope 校验。
func (s *Service) AddBinding(ctx context.Context, cmd AddBindingCommand) (*domain.Material, error) {
	ownerKind := domain.BindingOwnerKind(cmd.OwnerKind)
	switch ownerKind {
	case domain.OwnerVirtualIP, domain.OwnerProduct:
	default:
		return nil, fmt.Errorf("invalid owner_kind %q", cmd.OwnerKind)
	}
	// 同 scope 校验
	mat, err := s.repo.Get(ctx, cmd.MaterialID)
	if err != nil {
		return nil, fmt.Errorf("material %q: %w", cmd.MaterialID, err)
	}
	if s.ownerScope != nil {
		ownerScope, err := s.ownerScope.GetOwnerScope(ctx, cmd.OwnerKind, cmd.OwnerID)
		if err != nil {
			return nil, fmt.Errorf("get owner scope: %w", err)
		}
		if mat.Scope != ownerScope {
			return nil, fmt.Errorf("scope mismatch: material %q != owner %q", mat.Scope, ownerScope)
		}
	}
	b := &domain.Binding{
		MaterialID: cmd.MaterialID,
		OwnerKind:  ownerKind,
		OwnerID:    cmd.OwnerID,
		BoundBy:    cmd.BoundBy,
	}
	if err := s.bindings.AddBinding(ctx, b); err != nil {
		return nil, fmt.Errorf("add binding: %w", err)
	}
	return s.repo.Get(ctx, cmd.MaterialID)
}

// RemoveBinding 解绑。
func (s *Service) RemoveBinding(ctx context.Context, materialID, ownerKind, ownerID string) (*domain.Material, error) {
	kind := domain.BindingOwnerKind(ownerKind)
	if err := s.bindings.RemoveBinding(ctx, materialID, kind, ownerID); err != nil {
		return nil, fmt.Errorf("remove binding: %w", err)
	}
	return s.repo.Get(ctx, materialID)
}

// ListByOwner 给编排层组装 bundle 用。
func (s *Service) ListByOwner(ctx context.Context, ownerKind, ownerID string) ([]*domain.Material, error) {
	return s.bindings.ListByOwner(ctx, domain.BindingOwnerKind(ownerKind), ownerID)
}

// —— helpers ——

// canRead 可见性：owner ∨ public ∨ team（caller teams 在 ctx 中，由 auth 中间件注入）。
// team 分支需 service 持有 BindingRepo 以查 grants —— material 无 grants 表，team 可见性靠 material_bindings 的 owner team。
// 简化：material 的 team 可见性 = caller 在 material.team_id 这个 team 里。
func (s *Service) canRead(m *domain.Material, callerID string, callerTeams []string) bool {
	if m.OwnerUserID == callerID {
		return true
	}
	if m.Visibility == domain.VisibilityPublic {
		return true
	}
	if m.Visibility == domain.VisibilityTeam && m.TeamID != "" {
		for _, t := range callerTeams {
			if t == m.TeamID {
				return true
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
