package generated_asset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	productdomain "media_agent/media_materials/biz/domain/product"
	vipdomain "media_agent/media_materials/biz/domain/virtual_ip"
)

// Service generated_asset 用例编排。产物由 workflow 落（Upsert）；前端只读 + 删。
type Service struct {
	repo     domain.Repo
	products interface {
		Get(context.Context, string) (*productdomain.Product, error)
	}
	virtualIPs interface {
		Get(context.Context, string) (*vipdomain.VirtualIp, error)
	}
}

func NewService(repo domain.Repo, products interface {
	Get(context.Context, string) (*productdomain.Product, error)
}, virtualIPs interface {
	Get(context.Context, string) (*vipdomain.VirtualIp, error)
}) *Service {
	return &Service{repo: repo, products: products, virtualIPs: virtualIPs}
}

// Upsert 由 media_workflow 调用：落产物 + 生成记录字段。
// asset_kind 校验（resource_type=ASSET）由 workflow 侧 BatchGetAssetKind 已做；此处只兜底非空。
func (s *Service) Upsert(ctx context.Context, cmd UpsertCommand) (*domain.GeneratedAsset, error) {
	if strings.TrimSpace(cmd.AssetKind) == "" {
		return nil, errors.New("asset_kind is required")
	}
	if strings.TrimSpace(cmd.URL) == "" && strings.TrimSpace(cmd.Content) == "" {
		return nil, errors.New("url or content is required")
	}
	if cmd.JobID == "" || cmd.StepRunID == "" || cmd.TemplateID == "" {
		return nil, errors.New("job_id / step_run_id / template_id are required")
	}
	if existing, err := s.repo.GetByStepRun(ctx, cmd.StepRunID); err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup existing generated_asset: %w", err)
	}
	var organizationID string
	if cmd.SourceProductID != "" {
		product, err := s.products.Get(ctx, cmd.SourceProductID)
		if err != nil {
			return nil, fmt.Errorf("resolve source product: %w", err)
		}
		organizationID = product.OrganizationID
	} else if cmd.SourceVirtualIPID != "" {
		vip, err := s.virtualIPs.Get(ctx, cmd.SourceVirtualIPID)
		if err != nil {
			return nil, fmt.Errorf("resolve source virtual IP: %w", err)
		}
		organizationID = vip.OrganizationID
	}
	if organizationID == "" {
		return nil, errors.New("source product or virtual IP is required to resolve organization")
	}
	vis := domain.Visibility(cmd.Visibility)
	if vis == "" {
		vis = domain.VisibilityPrivate
	}
	a := &domain.GeneratedAsset{
		ID:             "gen_" + uuid.NewString(),
		OrganizationID: organizationID,
		Scope:          cmd.Scope, Name: cmd.Name, AssetKind: cmd.AssetKind,
		URL: cmd.URL, Content: cmd.Content, OwnerUserID: cmd.OwnerUserID,
		Visibility: vis, TeamID: cmd.TeamID,
		JobID: cmd.JobID, StepRunID: cmd.StepRunID, TemplateID: cmd.TemplateID,
		TemplateVersion:   cmd.TemplateVersion,
		SourceVirtualIPID: cmd.SourceVirtualIPID, SourceProductID: cmd.SourceProductID,
	}
	if err := s.repo.Save(ctx, a); err != nil {
		return nil, fmt.Errorf("save generated_asset: %w", err)
	}
	return s.repo.Get(ctx, a.ID)
}

func (s *Service) Get(ctx context.Context, id string) (*domain.GeneratedAsset, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("generated_asset %q: %w", id, err)
	}
	return a, nil
}

func (s *Service) List(ctx context.Context, q ListQuery) ([]*domain.GeneratedAsset, int, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	return s.repo.List(ctx, q)
}

func (s *Service) ListBySource(ctx context.Context, sourceKind, sourceID string) ([]*domain.GeneratedAsset, error) {
	if sourceKind != "virtual_ip" && sourceKind != "product" {
		return nil, fmt.Errorf("invalid source_kind %q", sourceKind)
	}
	return s.repo.ListBySource(ctx, sourceKind, sourceID)
}

func (s *Service) Delete(ctx context.Context, id string) (*domain.GeneratedAsset, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return nil, fmt.Errorf("delete generated_asset: %w", err)
	}
	return a, nil
}
