package material

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"

	domain "media_agent/media_materials/biz/domain/material"
	"media_agent/media_materials/biz/shared"
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

// EndBindingsForOwner 失效 owner（virtual_ip / product）的全部生效 material_bindings。
// 实现 shared.BindingEnder，注入到 virtual_ip / product service，在 owner 软删时调用
// （跨 domain 写用接口注入，避免 service 跨层 import，参照 OwnerScopeLookup 模式）。
func (s *Service) EndBindingsForOwner(ctx context.Context, ownerKind, ownerID string) error {
	return s.bindings.EndBindings(ctx, ownerKind, ownerID)
}

// Create 创建 material。
func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Material, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, errors.New("material name is required")
	}
	if strings.TrimSpace(cmd.OrganizationID) == "" {
		return nil, errors.New("material organization_id is required")
	}
	if strings.TrimSpace(cmd.ObjectPath) == "" && strings.TrimSpace(cmd.Content) == "" {
		return nil, errors.New("material object_path or content is required")
	}
	if err := validObjectPath(cmd.ObjectPath); err != nil {
		return nil, err
	}
	metadata, err := normalizedJSON(cmd.Metadata, "{}")
	if err != nil {
		return nil, fmt.Errorf("material metadata: %w", err)
	}
	if !shared.IsKnownScope(cmd.Scope) {
		return nil, errors.New("material scope is not supported")
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
		ID: "mat_" + uuid.NewString(), OrganizationID: cmd.OrganizationID,
		Scope: cmd.Scope, Name: name, Description: cmd.Description, Prompt: cmd.Prompt,
		AssetKind: cmd.AssetKind, ObjectPath: cmd.ObjectPath, Content: cmd.Content, Metadata: metadata,
		OwnerUserID: cmd.OwnerUserID, Visibility: vis, TeamID: cmd.TeamID,
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
	q.CallerID = callerID
	q.CallerTeamIDs = callerTeams
	return s.repo.List(ctx, q)
}

// Update 改可变字段。仅 owner。
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Material, error) {
	m, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if m.OrganizationID != cmd.OrganizationID {
		return nil, errors.New("material not found")
	}
	if m.OwnerUserID != cmd.CallerID {
		return nil, errors.New("only owner can update material")
	}
	if cmd.Name != "" {
		m.Name = strings.TrimSpace(cmd.Name)
	}
	m.Description = cmd.Description
	m.Prompt = cmd.Prompt
	if err := validObjectPath(cmd.ObjectPath); err != nil {
		return nil, err
	}
	m.ObjectPath = cmd.ObjectPath
	m.Content = cmd.Content
	if m.ObjectPath == "" && strings.TrimSpace(m.Content) == "" {
		return nil, errors.New("material object_path or content is required")
	}
	metadata, err := normalizedJSON(cmd.Metadata, "{}")
	if err != nil {
		return nil, fmt.Errorf("material metadata: %w", err)
	}
	m.Metadata = metadata
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
func (s *Service) Delete(ctx context.Context, id, organizationID, callerID string) (*domain.Material, error) {
	m, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.OrganizationID != organizationID {
		return nil, errors.New("material not found")
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

func validObjectPath(value string) error {
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../") {
		return errors.New("object_path must be a clean storage-relative path")
	}
	return nil
}

func normalizedJSON(value, fallback string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	if !json.Valid([]byte(value)) {
		return "", errors.New("must be valid JSON")
	}
	return value, nil
}
