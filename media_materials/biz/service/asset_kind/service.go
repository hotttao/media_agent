package asset_kind

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	domain "media_agent/media_materials/biz/domain/asset_kind"
)

// Service asset_kind 用例编排。持有 Repo。
// asset_kind 是全局注册表，无 scope、无 user ACL；CRUD 不开事务（单行操作）。
type Service struct {
	repo domain.Repo
}

func NewService(repo domain.Repo) *Service { return &Service{repo: repo} }

// sourceTableFor resource_type → 物理表（TEXT 无表）。
func sourceTableFor(rt domain.ResourceType) string {
	switch rt {
	case domain.ResourceTypeMaterial:
		return "materials"
	case domain.ResourceTypeAsset:
		return "generated_assets"
	case domain.ResourceTypeProduct:
		return "products"
	case domain.ResourceTypeVirtualIP:
		return "virtual_ips"
	case domain.ResourceTypeText:
		return ""
	}
	return ""
}

// Register 注册新 asset_kind（status=ENABLED；SYSTEM 只能由 seed 迁移创建）。
func (s *Service) Register(ctx context.Context, cmd RegisterCommand) (*domain.AssetKind, error) {
	rt, mt, err := parseTypes(cmd.ResourceType, cmd.MediaType)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, errors.New("asset_kind name is required")
	}
	// source_table：caller 传则校验，否则按 resource_type 推导
	st := strings.TrimSpace(cmd.SourceTable)
	if st == "" {
		st = sourceTableFor(rt)
	} else if st != sourceTableFor(rt) {
		return nil, fmt.Errorf("source_table %q mismatched with resource_type %q (expect %q)", st, rt, sourceTableFor(rt))
	}

	ak := &domain.AssetKind{
		Name:         name,
		ResourceType: rt,
		MediaType:    mt,
		SourceTable:  st,
		Description:  cmd.Description,
		Status:       domain.StatusEnabled,
	}
	if err := s.repo.Save(ctx, ak); err != nil {
		return nil, fmt.Errorf("save asset_kind: %w", err)
	}
	return s.repo.Get(ctx, name)
}

// Get 按 name 取。
func (s *Service) Get(ctx context.Context, name string) (*domain.AssetKind, error) {
	ak, err := s.repo.Get(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("asset_kind %q not found", name)
		}
		return nil, err
	}
	return ak, nil
}

// Lookup 返回原语类型（实现 material.AssetKindLookup 端口，避免跨 domain import）。
func (s *Service) Lookup(ctx context.Context, name string) (string, string, string, error) {
	ak, err := s.Get(ctx, name)
	if err != nil {
		return "", "", "", err
	}
	return string(ak.ResourceType), string(ak.MediaType), string(ak.Status), nil
}

// List 分页过滤。
func (s *Service) List(ctx context.Context, q ListQuery) ([]*domain.AssetKind, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return s.repo.List(ctx, q)
}

// Update 改 description / status。锁列（name/resource_type/media_type/source_table）不可改。
// SYSTEM：仅可改 description + status↔DISABLED。
func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.AssetKind, error) {
	ak, err := s.Get(ctx, cmd.Name)
	if err != nil {
		return nil, err
	}
	ak.Description = cmd.Description
	if cmd.Status != "" {
		st := domain.Status(cmd.Status)
		if !validStatus(st) {
			return nil, fmt.Errorf("invalid status %q", cmd.Status)
		}
		ak.Status = st
	}
	if err := s.repo.Update(ctx, ak); err != nil {
		return nil, fmt.Errorf("update asset_kind: %w", err)
	}
	return s.repo.Get(ctx, cmd.Name)
}

// Delete 删除。SYSTEM 拒绝；ENABLED 可删；in-use 校验。
func (s *Service) Delete(ctx context.Context, name string) (*domain.AssetKind, error) {
	ak, err := s.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if ak.Status == domain.StatusSystem {
		return nil, fmt.Errorf("asset_kind %q is SYSTEM, cannot delete", name)
	}
	// in-use 校验
	matCount, err := s.repo.CountMaterialsByAssetKind(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("check material references: %w", err)
	}
	genCount, err := s.repo.CountGeneratedAssetsByAssetKind(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("check generated_asset references: %w", err)
	}
	if matCount > 0 || genCount > 0 {
		return nil, fmt.Errorf("asset_kind %q is in use (%d materials, %d generated_assets)", name, matCount, genCount)
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return nil, fmt.Errorf("delete asset_kind: %w", err)
	}
	return ak, nil
}

// BatchGet 批量取（给 media_workflow 校验期用）。
// 返回 (kinds, not_found)。
func (s *Service) BatchGet(ctx context.Context, names []string) ([]*domain.AssetKind, []string) {
	ks, err := s.repo.BatchGet(ctx, names)
	if err != nil {
		return nil, dup(names) // 出错时全部视为 not_found
	}
	got := make(map[string]struct{}, len(ks))
	for _, k := range ks {
		got[k.Name] = struct{}{}
	}
	var missing []string
	for _, n := range names {
		if _, ok := got[n]; !ok {
			missing = append(missing, n)
		}
	}
	return ks, missing
}

// —— helpers ——

func parseTypes(rt, mt string) (domain.ResourceType, domain.MediaType, error) {
	R := domain.ResourceType(rt)
	switch R {
	case domain.ResourceTypeMaterial, domain.ResourceTypeAsset, domain.ResourceTypeProduct,
		domain.ResourceTypeVirtualIP, domain.ResourceTypeText:
	default:
		return "", "", fmt.Errorf("invalid resource_type %q", rt)
	}
	M := domain.MediaType(mt)
	switch M {
	case domain.MediaTypeImage, domain.MediaTypeText, domain.MediaTypeVideo:
	default:
		return "", "", fmt.Errorf("invalid media_type %q", mt)
	}
	return R, M, nil
}

func validStatus(s domain.Status) bool {
	switch s {
	case domain.StatusSystem, domain.StatusEnabled, domain.StatusDisabled:
		return true
	}
	return false
}

func dup(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}
