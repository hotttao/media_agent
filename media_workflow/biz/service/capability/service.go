package capability

import (
	"context"
	"errors"
	domain "media_agent/media_workflow/biz/domain/capability"
	"sort"
	"strings"
)

var ErrUnauthenticated = errors.New("authentication required")
var ErrInvalidQuery = errors.New("invalid capability query")

type Service struct{ repo domain.Repo }

func NewService(repo domain.Repo) *Service { return &Service{repo: repo} }

type ListQuery struct {
	Provider, GenerationMethod, GenerationType, ExecutionTarget string
	Page, PageSize                                              int
}
type Page struct {
	Items                 []*domain.Capability
	Total, Page, PageSize int
}

func publicContract(item *domain.Capability) bool {
	return item != nil && item.Status == "published" && item.GenerationMethod != "" && item.GenerationType != "" && item.OutputMediaType.Valid() && item.ValidateContract() == nil
}
func (s *Service) List(ctx context.Context, caller string, query ListQuery) (*Page, error) {
	if caller == "" {
		return nil, ErrUnauthenticated
	}
	if query.Page < 0 || query.PageSize < 0 || query.PageSize > 200 || (query.ExecutionTarget != "" && !domain.ExecutionTarget(query.ExecutionTarget).Valid()) || (query.GenerationMethod != "" && domain.GenerationMethod(query.GenerationMethod).OutputMedia() == "") || (query.GenerationType != "" && query.GenerationType != string(domain.FirstFramePrompt) && query.GenerationType != string(domain.MultiImagePrompt) && query.GenerationType != string(domain.TextPrompt)) {
		return nil, ErrInvalidQuery
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.Page > 1_000_000 {
		return nil, ErrInvalidQuery
	}
	rows, err := s.repo.ListPublished(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*domain.Capability, 0, len(rows))
	for _, row := range rows {
		if !publicContract(row) || (query.Provider != "" && row.Provider != query.Provider) || (query.GenerationMethod != "" && string(row.GenerationMethod) != query.GenerationMethod) || (query.GenerationType != "" && string(row.GenerationType) != query.GenerationType) || (query.ExecutionTarget != "" && string(row.ExecutionTarget) != query.ExecutionTarget) {
			continue
		}
		items = append(items, row)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			return items[i].Version < items[j].Version
		}
		return items[i].Key < items[j].Key
	})
	total := len(items)
	offset := (query.Page - 1) * query.PageSize
	if offset > total {
		offset = total
	}
	end := offset + query.PageSize
	if end > total {
		end = total
	}
	return &Page{Items: items[offset:end], Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}
func (s *Service) GetVersion(ctx context.Context, caller, key string, version int) (*domain.Capability, error) {
	if caller == "" {
		return nil, ErrUnauthenticated
	}
	if strings.TrimSpace(key) == "" || version < 1 {
		return nil, ErrInvalidQuery
	}
	item, err := s.repo.Get(ctx, key, version)
	if err != nil {
		return nil, err
	}
	if !publicContract(item) {
		return nil, domain.ErrCapabilityNotFound
	}
	return item, nil
}
