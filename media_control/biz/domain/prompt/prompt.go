package prompt

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound    = errors.New("prompt not found")
	ErrConflict    = errors.New("prompt version conflict")
	ErrArchived    = errors.New("prompt is archived")
	ErrForbidden   = errors.New("prompt access denied")
	ErrInvalid     = errors.New("invalid prompt")
	ErrUnavailable = errors.New("prompt authorization unavailable")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Grant struct{ TeamID, Permission string }
type Prompt struct {
	ID, OrganizationID, OwnerUserID, Name, Content, Visibility string
	Status                                                     Status
	Version                                                    int64
	Grants                                                     []Grant
	CreatedAt, UpdatedAt                                       time.Time
}
type Version struct {
	Version       int64
	Name, Content string
	CreatedAt     time.Time
}
type Repository interface {
	Get(context.Context, string) (*Prompt, error)
	List(context.Context, string, string, []string) ([]Prompt, error)
	Save(context.Context, *Prompt, int64) error
	Versions(context.Context, string) ([]Version, error)
	GetVersion(context.Context, string, int64) (*Version, error)
	ListUnassigned(context.Context, string) ([]Prompt, error)
	AssignOrganization(context.Context, *Prompt, int64) error
}

func Validate(name, content string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 || len(content) > 1<<20 {
		return "", "", ErrInvalid
	}
	return name, content, nil
}
func ValidateACL(visibility string, grants []Grant) error {
	if visibility != "private" && visibility != "team" && visibility != "public" {
		return ErrInvalid
	}
	if visibility == "private" && len(grants) != 0 || visibility == "team" && len(grants) == 0 || len(grants) > 100 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, g := range grants {
		if strings.TrimSpace(g.TeamID) == "" || seen[g.TeamID] || g.Permission != "viewer" && g.Permission != "editor" {
			return ErrInvalid
		}
		seen[g.TeamID] = true
	}
	return nil
}

// Permission requires verified active organization membership.
func (p Prompt) Permission(user string, teams []string) string {
	if p.OwnerUserID == user {
		return "owner"
	}
	permission := "none"
	if p.Visibility == "public" {
		permission = "public"
	}
	if p.Visibility == "private" {
		return permission
	}
	for _, g := range p.Grants {
		for _, team := range teams {
			if g.TeamID == team {
				if g.Permission == "editor" {
					return "editor"
				}
				permission = "viewer"
			}
		}
	}
	return permission
}
