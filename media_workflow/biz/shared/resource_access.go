package shared

import (
	"context"
	"errors"
)

var ErrResourceAuthorityUnavailable = errors.New("resource authorization unavailable")
var ErrResourceAccessDenied = errors.New("resource access denied")

// ResourceFacts are current authoritative facts, not claims supplied by clients.
type ResourceFacts struct {
	OrganizationIDs         []string
	TeamIDs                 []string
	WorkflowTeamIDs         []string
	WorkflowOrganizationIDs []string
	WorkspaceIDs            []string
}

type ResourceAuthority interface {
	Facts(context.Context, string) (ResourceFacts, error)
	TeamOrganization(context.Context, string, string) (string, error)
	GetWorkspaceForUser(context.Context, string, string) (*ResourceWorkspace, error)
}

type ResourceWorkspace struct{ ID, OrganizationID, TeamID string }

func HasResourceID(ids []string, id string) bool {
	for _, value := range ids {
		if value == id && id != "" {
			return true
		}
	}
	return false
}

type resourceFactsKey struct{}

type resourceOrganizationKey struct{}

func WithResourceOrganization(ctx context.Context, org string) context.Context {
	return context.WithValue(ctx, resourceOrganizationKey{}, org)
}
func ResourceOrganization(ctx context.Context) string {
	v, _ := ctx.Value(resourceOrganizationKey{}).(string)
	return v
}

type resourceFactsSnapshot struct {
	User  string
	Facts ResourceFacts
}

// PrepareResourceFacts freezes only this request's authority read, allowing
// local transactions to recheck ACL without doing remote IO under DB locks.
func PrepareResourceFacts(ctx context.Context, user string, source ResourceAuthority) (context.Context, ResourceFacts, error) {
	if cached, ok := ctx.Value(resourceFactsKey{}).(resourceFactsSnapshot); ok && cached.User == user {
		return ctx, cached.Facts, nil
	}
	if source == nil {
		return ctx, ResourceFacts{}, ErrResourceAuthorityUnavailable
	}
	f, err := source.Facts(ctx, user)
	if err != nil {
		return ctx, f, err
	}
	return context.WithValue(ctx, resourceFactsKey{}, resourceFactsSnapshot{user, f}), f, nil
}
