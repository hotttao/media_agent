package product

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domain "media_agent/media_materials/biz/domain/product"
	"media_agent/media_materials/biz/shared"
)

// Service product 用例编排。无 user ACL（任意已认证 user 可 CRUD）。
type Service struct {
	repo         domain.Repo
	bindingEnder shared.BindingEnder // 注入：owner 软删时失效 material_bindings
}

func NewService(repo domain.Repo) *Service { return &Service{repo: repo} }

// SetBindingEnder 注入 material BindingEnder（owner 软删时失效其 material_bindings）。
func (s *Service) SetBindingEnder(ender shared.BindingEnder) { s.bindingEnder = ender }

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (*domain.Product, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return nil, errors.New("product name is required")
	}
	if !shared.IsKnownScope(cmd.Scope) {
		return nil, errors.New("product scope is not supported")
	}
	p := &domain.Product{
		ID:    "prd_" + uuid.NewString(),
		Scope: cmd.Scope, Name: name,
		Description: cmd.Description, TargetAudience: cmd.TargetAudience,
		ProductDetails: cmd.ProductDetails, DisplayActions: cmd.DisplayActions,
		Status: domain.StatusActive, CreatedBy: cmd.CreatedBy,
	}
	if err := s.repo.Save(ctx, p); err != nil {
		return nil, fmt.Errorf("save product: %w", err)
	}
	return s.repo.Get(ctx, p.ID)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("product %q: %w", id, err)
	}
	return p, nil
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]*domain.Product, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return s.repo.List(ctx, q)
}

func (s *Service) Update(ctx context.Context, cmd UpdateCommand) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if cmd.Name != "" {
		p.Name = cmd.Name
	}
	p.Description = cmd.Description
	p.TargetAudience = cmd.TargetAudience
	p.ProductDetails = cmd.ProductDetails
	p.DisplayActions = cmd.DisplayActions
	p.UpdatedBy = cmd.UpdatedBy
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}
	return s.repo.Get(ctx, cmd.ID)
}

func (s *Service) Archive(ctx context.Context, id string) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Status = domain.StatusArchived
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("archive product: %w", err)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete product: %w", err)
	}
	if s.bindingEnder != nil {
		if err := s.bindingEnder.EndBindingsForOwner(ctx, "product", id); err != nil {
			return nil, fmt.Errorf("end bindings for product: %w", err)
		}
	}
	return p, nil
}

// GetOwnerScope 实现 material.OwnerScopeLookup 端口。
func (s *Service) GetOwnerScope(ctx context.Context, ownerKind, ownerID string) (string, error) {
	if ownerKind != "product" {
		return "", fmt.Errorf("owner_kind %q not handled by product service", ownerKind)
	}
	p, err := s.repo.Get(ctx, ownerID)
	if err != nil {
		return "", err
	}
	return p.Scope, nil
}
