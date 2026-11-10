package application

import (
	"context"
	"errors"
	d "media_agent/media_auth/biz/domain/application"
)

func (s *Service) SetMCPTarget(ctx context.Context, role, app, id, origin string, remove bool) error {
	if role != "admin" {
		return d.ErrForbidden
	}
	if !d.ValidID(id) {
		return d.ErrInvalid
	}
	r, ok := s.repo.(d.CallerRepository)
	if !ok {
		return d.ErrForbidden
	}
	canonical, valid := d.CanonicalOrigin(origin)
	if !remove && (!valid || canonical != origin) {
		return d.ErrInvalid
	}
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, e := s.repo.Get(ctx, app, true); e != nil {
			return e
		}
		if remove {
			return r.DeleteTarget(ctx, app, id)
		}
		return r.PutTarget(ctx, d.MCPTarget{ApplicationID: app, DefinitionID: id, Origin: origin})
	})
}
func (s *Service) ResolveCaller(ctx context.Context, id, endpoint, user, org, team string) (d.Profile, bool, error) {
	r, ok := s.repo.(d.CallerRepository)
	if !ok || s.business == nil {
		return d.Profile{}, false, d.ErrForbidden
	}
	target, e := r.Target(ctx, id)
	if errors.Is(e, d.ErrNotFound) {
		return d.Profile{}, false, nil
	}
	if e != nil {
		return d.Profile{}, false, e
	}
	origin, valid := d.CanonicalOrigin(endpoint)
	if !valid || origin != target.Origin {
		return d.Profile{}, false, d.ErrForbidden
	}
	p, e := r.CallerProfile(ctx, target.ApplicationID, user)
	if e != nil {
		return p, false, e
	}
	g, e := s.business.Grant(ctx, p.ApplicationID, team)
	if errors.Is(e, d.ErrNotFound) {
		e = d.ErrForbidden
	}
	if e != nil {
		return p, false, e
	}
	if g.OrganizationID != org || !d.ContainsScopes(g.Scopes, []string{"session.chat"}) {
		return p, false, d.ErrForbidden
	}
	if e = s.teamAllowed(ctx, user, org, team, []string{"session.chat"}); e != nil {
		return p, false, e
	}
	return p, true, nil
}
