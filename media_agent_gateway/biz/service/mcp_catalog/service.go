package mcp_catalog

import (
	"context"
	"github.com/google/uuid"
	d "media_agent/media_agent_gateway/biz/domain/mcp_catalog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}
type Service struct {
	repo  d.Repository
	tx    TxRunner
	hosts map[string]bool
}

func New(repo d.Repository, tx TxRunner, hosts []string) *Service {
	allow := map[string]bool{}
	for _, host := range hosts {
		allow[host] = true
	}
	return &Service{repo, tx, allow}
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)

func (s *Service) valid(v d.Definition) bool {
	if v.OrganizationID != "" && (v.Transport == "platform" || v.AuthenticationType == "session_context") {
		return false
	}
	if v.ID == "platform" && v.Transport != "platform" {
		return false
	}
	if !slug.MatchString(v.ID) || strings.Contains(v.ID, "__") || !slug.MatchString(v.ServiceName) || !slices.Contains([]string{"enabled", "disabled", "retired"}, v.Status) {
		return false
	}
	if v.Transport == "platform" {
		return v.ID == "platform" && v.ServiceName == "platform" && v.Endpoint == "platform://builtin" && v.AuthenticationType == "session_context"
	}
	if !slices.Contains([]string{"streamable_http", "sse"}, v.Transport) || !slices.Contains([]string{"session_context", "oauth", "none", "bearer", "api_key"}, v.AuthenticationType) {
		return false
	}
	u, e := url.Parse(v.Endpoint)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.Fragment == "" && u.RawQuery == "" && s.hosts[u.Host]
}
func (s *Service) Put(ctx context.Context, v d.Definition, expected int64, admin bool) (*d.Definition, error) {
	if !admin || v.OrganizationID != "" {
		return nil, d.ErrForbidden
	}
	return s.PutScoped(ctx, v, expected, "")
}

// PutScoped accepts an organization authorized by the application workflow.
// Empty organization is reserved for platform administration.
func (s *Service) PutScoped(ctx context.Context, v d.Definition, expected int64, organization string) (*d.Definition, error) {
	if v.OrganizationID != organization {
		return nil, d.ErrForbidden
	}
	if expected < 0 || !s.valid(v) {
		return nil, d.ErrInvalid
	}
	var result *d.Definition
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		previous, e := s.repo.Get(ctx, v.ID, true)
		if e != nil && e != d.ErrNotFound {
			return e
		}
		now := time.Now().UTC()
		if previous == nil {
			if expected != 0 {
				return d.ErrConflict
			}
			v.CreatedAt = now
		} else {
			if previous.OrganizationID != organization {
				return d.ErrForbidden
			}
			if previous.Version != expected || previous.Status == "retired" && v.Status != "retired" {
				return d.ErrConflict
			}
			v.CreatedAt = previous.CreatedAt
		}
		v.Version = expected + 1
		v.UpdatedAt = now
		if e = s.repo.Save(ctx, &v, expected); e != nil {
			return e
		}
		if e = s.repo.Changed(ctx, d.Event{ID: uuid.NewString(), DefinitionID: v.ID, Version: v.Version, CreatedAt: now}); e != nil {
			return e
		}
		result = &v
		return nil
	})
	return result, err
}
func (s *Service) Get(ctx context.Context, id string) (*d.Definition, error) {
	return s.repo.Get(ctx, id, false)
}
func (s *Service) Version(ctx context.Context, id string, version int64) (*d.Definition, error) {
	return s.repo.Version(ctx, id, version)
}
func (s *Service) List(ctx context.Context, after string, limit int) ([]d.Definition, error) {
	if limit < 1 || limit > 200 {
		return nil, d.ErrInvalid
	}
	return s.repo.List(ctx, after, limit)
}

func (s *Service) PendingEvents(ctx context.Context, limit int) ([]d.Event, error) {
	repo, ok := s.repo.(d.EventRepository)
	if !ok || limit < 1 || limit > 200 {
		return nil, d.ErrInvalid
	}
	return repo.PendingEvents(ctx, limit)
}
func (s *Service) CompleteEvent(ctx context.Context, id string) error {
	repo, ok := s.repo.(d.EventRepository)
	if !ok {
		return d.ErrInvalid
	}
	return repo.CompleteEvent(ctx, id)
}
func (s *Service) DeferEvent(ctx context.Context, id string) error {
	repo, ok := s.repo.(d.EventRepository)
	if !ok {
		return d.ErrInvalid
	}
	return repo.DeferEvent(ctx, id)
}

func (s *Service) ListVisible(ctx context.Context, organization, after string, limit int) ([]d.Definition, error) {
	if limit < 1 || limit > 200 {
		return nil, d.ErrInvalid
	}
	repo, ok := s.repo.(interface {
		ListVisible(context.Context, string, string, int) ([]d.Definition, error)
	})
	if !ok {
		return nil, d.ErrInvalid
	}
	return repo.ListVisible(ctx, organization, after, limit)
}
