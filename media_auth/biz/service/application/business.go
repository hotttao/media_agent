package application

import (
	"context"
	"errors"
	d "media_agent/media_auth/biz/domain/application"
	"strings"
	"time"
)

func (s *Service) WithBusiness(repo d.BusinessRepository, teams d.TeamPermissions) *Service {
	s.business = repo
	s.teams = teams
	return s
}
func (s *Service) SetGrant(ctx context.Context, role, app, team string, scopes []string, remove bool) (d.TeamGrant, error) {
	if role != "admin" {
		return d.TeamGrant{}, d.ErrForbidden
	}
	if s.business == nil {
		return d.TeamGrant{}, errors.New("application authority unavailable")
	}
	if team == "" || (!remove && !d.ValidScopes(scopes)) {
		return d.TeamGrant{}, d.ErrInvalid
	}
	var g d.TeamGrant
	e := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		a, e := s.repo.Get(ctx, app, true)
		if e != nil {
			return e
		}
		if remove {
			if e = s.business.DeleteGrant(ctx, app, team); e != nil {
				return e
			}
		} else {
			org, active, e := s.business.TeamTarget(ctx, team)
			if e != nil {
				return e
			}
			if !active {
				return d.ErrForbidden
			}
			g = d.TeamGrant{ApplicationID: app, TeamID: team, OrganizationID: org, Scopes: scopes}
			if e = s.business.PutGrant(ctx, g); e != nil {
				return e
			}
		}
		_, e = s.repo.SetStatus(ctx, app, a.Status)
		return e
	})
	return g, e
}
func (s *Service) teamAllowed(ctx context.Context, user, org, team string, scopes []string) error {
	if s.teams == nil {
		return errors.New("team authority unavailable")
	}
	for _, scope := range scopes {
		permission := d.ScopePermission(scope)
		if permission == "" {
			return d.ErrForbidden
		}
		current, allowed, e := s.teams.Allowed(ctx, user, team, permission)
		if e != nil {
			return e
		}
		if !allowed || current != org {
			return d.ErrForbidden
		}
	}
	return nil
}
func (s *Service) IssueBusiness(ctx context.Context, a d.Application, user, external string, version int64, team string, scopes []string) (string, error) {
	if team == "" && len(scopes) == 0 {
		return s.Issue(ctx, a, user, external, version)
	}
	if s.business == nil || !d.ValidScopes(scopes) || team == "" {
		return "", d.ErrInvalid
	}
	g, e := s.business.Grant(ctx, a.ID, team)
	if errors.Is(e, d.ErrNotFound) {
		return "", d.ErrForbidden
	}
	if e != nil {
		return "", e
	}
	if !d.ContainsScopes(g.Scopes, scopes) {
		return "", d.ErrForbidden
	}
	if e = s.teamAllowed(ctx, user, g.OrganizationID, team, scopes); e != nil {
		return "", e
	}
	raw, e := random("mac_")
	if e != nil {
		return "", e
	}
	e = s.repo.SaveCredential(ctx, d.Credential{Hash: digest(raw), ApplicationID: a.ID, UserID: user, ExternalID: external, ApplicationVersion: a.CredentialVersion, AuthVersion: version, TeamID: team, OrganizationID: g.OrganizationID, Scopes: scopes, ExpiresAt: time.Now().Add(d.AccessTTL)})
	if e != nil {
		return "", e
	}
	return raw, nil
}
func (s *Service) VerifyBusiness(ctx context.Context, raw, scope string) (d.BusinessCredential, error) {
	if !strings.HasPrefix(raw, "mac_") || len(raw) != 68 || s.business == nil {
		return d.BusinessCredential{}, d.ErrUnauthorized
	}
	c, e := s.business.ResolveBusinessCredential(ctx, digest(raw))
	if e != nil {
		return d.BusinessCredential{}, e
	}
	if !d.ContainsScopes(c.Scopes, []string{scope}) {
		return d.BusinessCredential{}, d.ErrForbidden
	}
	if e = s.teamAllowed(ctx, c.UserID, c.OrganizationID, c.TeamID, []string{scope}); e != nil {
		return d.BusinessCredential{}, e
	}
	return c, nil
}
