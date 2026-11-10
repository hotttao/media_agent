// Package application owns trusted application registration and scoped short-lived credentials.
package application

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalid      = errors.New("invalid application input")
	ErrUnauthorized = errors.New("invalid application credential")
	ErrForbidden    = errors.New("application operation forbidden")
	ErrNotFound     = errors.New("application resource not found")
	ErrConflict     = errors.New("application already exists")
)

const AccessTTL = time.Hour

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func ValidID(id string) bool { return slug.MatchString(id) }
func ValidProfile(externalID, name, email string) bool {
	return strings.TrimSpace(externalID) != "" && len(externalID) <= 256 && !strings.ContainsAny(externalID, "/\\\x00") && strings.TrimSpace(name) != "" && len(name) <= 256 && len(email) <= 320
}

type Application struct {
	ID, Name, SecretHash, Status, CreatedBy string
	CredentialVersion                       int64
	CreatedAt                               time.Time
}
type Credential struct {
	TeamID, OrganizationID                  string
	Scopes                                  []string
	Hash, ApplicationID, UserID, ExternalID string
	ApplicationVersion, AuthVersion         int64
	ExpiresAt                               time.Time
}

// Profile is an application-scoped read model, never a general User session or Actor.
type Profile struct{ ApplicationID, UserID, ExternalID, DisplayName, Email string }
type Repository interface {
	Create(context.Context, Application) (Application, error)
	List(context.Context) ([]Application, error)
	Get(context.Context, string, bool) (Application, error)
	SetStatus(context.Context, string, string) (Application, error)
	Rotate(context.Context, string, string) (Application, error)
	SaveCredential(context.Context, Credential) error
	ResolveCredential(context.Context, string) (Profile, error)
}
type TxRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}

type TeamGrant struct {
	ApplicationID, TeamID, OrganizationID string
	Scopes                                []string
}
type BusinessCredential struct {
	Profile
	TeamID, OrganizationID string
	Scopes                 []string
	ExpiresAt              time.Time
}
type BusinessRepository interface {
	TeamTarget(context.Context, string) (string, bool, error)
	PutGrant(context.Context, TeamGrant) error
	DeleteGrant(context.Context, string, string) error
	Grant(context.Context, string, string) (TeamGrant, error)
	ResolveBusinessCredential(context.Context, string) (BusinessCredential, error)
}

// TeamPermissions reads the current membership and entitlement authority.
type TeamPermissions interface {
	Allowed(context.Context, string, string, string) (string, bool, error)
}

func ValidScopes(scopes []string) bool {
	if len(scopes) == 0 || len(scopes) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range scopes {
		if seen[s] {
			return false
		}
		seen[s] = true
		switch s {
		case "workspace.read", "workspace.ensure", "session.chat":
		default:
			return false
		}
	}
	return true
}
func ScopePermission(scope string) string {
	switch scope {
	case "workspace.read":
		return "view"
	case "workspace.ensure":
		return "create_workspace"
	case "session.chat":
		return "manage"
	}
	return ""
}
func ContainsScopes(allowed, requested []string) bool {
	m := map[string]bool{}
	for _, s := range allowed {
		m[s] = true
	}
	for _, s := range requested {
		if !m[s] {
			return false
		}
	}
	return true
}
