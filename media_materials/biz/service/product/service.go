package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
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
	if strings.TrimSpace(cmd.OrganizationID) == "" {
		return nil, errors.New("product organization_id is required")
	}
	if !shared.IsKnownScope(cmd.Scope) {
		return nil, errors.New("product scope is not supported")
	}
	displayActions, tags, external, metadata, err := normalizeProductJSON(cmd.DisplayActions, cmd.Tags, cmd.ExternalCommerce, cmd.Metadata)
	if err != nil {
		return nil, err
	}
	images, err := buildImages("", cmd.OrganizationID, cmd.Images)
	if err != nil {
		return nil, err
	}
	p := &domain.Product{
		ID: "prd_" + uuid.NewString(), OrganizationID: cmd.OrganizationID, Scope: cmd.Scope, Name: name,
		Description: cmd.Description, TargetAudience: cmd.TargetAudience,
		ProductDetails: cmd.ProductDetails, DisplayActions: displayActions, Tags: tags,
		ExternalCommerce: external, Metadata: metadata, Images: images,
		Status: domain.StatusActive, CreatedBy: cmd.CreatedBy,
	}
	for i := range p.Images {
		p.Images[i].ProductID = p.ID
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
	if p.OrganizationID != cmd.OrganizationID {
		return nil, errors.New("product not found")
	}
	if cmd.Name != "" {
		p.Name = cmd.Name
	}
	p.Description = cmd.Description
	p.TargetAudience = cmd.TargetAudience
	p.ProductDetails = cmd.ProductDetails
	displayActions, tags, external, metadata, err := normalizeProductJSON(cmd.DisplayActions, cmd.Tags, cmd.ExternalCommerce, cmd.Metadata)
	if err != nil {
		return nil, err
	}
	p.DisplayActions, p.Tags, p.ExternalCommerce, p.Metadata = displayActions, tags, external, metadata
	images, err := buildImages(p.ID, p.OrganizationID, cmd.Images)
	if err != nil {
		return nil, err
	}
	p.Images = images
	p.UpdatedBy = cmd.UpdatedBy
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update product: %w", err)
	}
	return s.repo.Get(ctx, cmd.ID)
}

func (s *Service) Archive(ctx context.Context, id, organizationID string) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.OrganizationID != organizationID {
		return nil, errors.New("product not found")
	}
	p.Status = domain.StatusArchived
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("archive product: %w", err)
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id, organizationID string) (*domain.Product, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.OrganizationID != organizationID {
		return nil, errors.New("product not found")
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

func normalizeProductJSON(actions, tags, external, metadata string) (string, string, string, string, error) {
	values := []*string{&actions, &tags, &external, &metadata}
	fallbacks := []string{"[]", "[]", "{}", "{}"}
	labels := []string{"display_actions", "tags", "external_commerce", "metadata"}
	for i, value := range values {
		if strings.TrimSpace(*value) == "" {
			*value = fallbacks[i]
		}
		if !json.Valid([]byte(*value)) {
			return "", "", "", "", fmt.Errorf("%s must be valid JSON", labels[i])
		}
	}
	return actions, tags, external, metadata, nil
}

func buildImages(productID, organizationID string, commands []ImageCommand) ([]domain.Image, error) {
	images := make([]domain.Image, len(commands))
	orders := make(map[int]bool, len(commands))
	mainCount := 0
	for i, command := range commands {
		if command.ObjectPath == "" || strings.HasPrefix(command.ObjectPath, "/") || path.Clean(command.ObjectPath) != command.ObjectPath || strings.HasPrefix(command.ObjectPath, "../") {
			return nil, errors.New("product image object_path must be a clean storage-relative path")
		}
		if command.SortOrder < 0 || orders[command.SortOrder] {
			return nil, errors.New("product image sort_order must be unique and non-negative")
		}
		orders[command.SortOrder] = true
		if command.IsMain {
			mainCount++
		}
		images[i] = domain.Image{ID: "pimg_" + uuid.NewString(), OrganizationID: organizationID,
			ProductID: productID, ObjectPath: command.ObjectPath, IsMain: command.IsMain, SortOrder: command.SortOrder}
	}
	if mainCount > 1 {
		return nil, errors.New("product can have at most one main image")
	}
	return images, nil
}
