package skill

import (
	"context"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("skill not found")
	ErrConflict      = errors.New("skill version conflict")
	ErrArchived      = errors.New("skill is archived")
	ErrForbidden     = errors.New("skill access denied")
	ErrInvalid       = errors.New("invalid skill")
	ErrInvalidStatus = errors.New("invalid skill status")
	ErrUnavailable   = errors.New("skill authorization unavailable")
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusArchived Status = "archived"
)

type Grant struct{ TeamID, Permission string }
type GitSource struct {
	URL, Branch, Subpath, Commit string
	EncryptedToken               []byte
}
type Skill struct {
	ID, OrganizationID, OwnerUserID, Name, Description, Visibility, ContentPath, Checksum, DraftChecksum, SourceKind string
	Source                                                                                                           GitSource
	Status                                                                                                           Status
	Version, PublishedVersion                                                                                        int64
	Grants                                                                                                           []Grant
	CreatedAt, UpdatedAt                                                                                             time.Time
}
type Version struct {
	SkillID                                  string
	Version                                  int64
	Checksum, Slug, Description, PublishedBy string
	CreatedAt                                time.Time
}
type Repository interface {
	Get(context.Context, string) (*Skill, error)
	List(context.Context, string, string, []string) ([]Skill, error)
	Save(context.Context, *Skill, int64) error
	CreateVersion(context.Context, *Version) error
	Versions(context.Context, string) ([]Version, error)
	GetVersion(context.Context, string, int64) (*Version, error)
	ListUnassigned(context.Context, string) ([]Skill, error)
	AssignOrganization(context.Context, *Skill, int64) error
}

var checksum = regexp.MustCompile(`^[0-9a-f]{64}$`)
var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func IsChecksum(value string) bool { return checksum.MatchString(value) }
func IsSlug(value string) bool     { return len(value) <= 64 && slug.MatchString(value) }
func Validate(name, description string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 || len(description) > 4096 {
		return "", "", ErrInvalid
	}
	return name, description, nil
}
func ParseStatus(value string) (Status, error) {
	switch Status(strings.ToLower(strings.TrimSpace(value))) {
	case StatusActive:
		return StatusActive, nil
	case StatusDisabled:
		return StatusDisabled, nil
	default:
		return "", ErrInvalidStatus
	}
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
		if g.TeamID == "" || seen[g.TeamID] || g.Permission != "viewer" && g.Permission != "editor" {
			return ErrInvalid
		}
		seen[g.TeamID] = true
	}
	return nil
}
func (s Skill) Permission(user string, teams []string) string {
	if user == s.OwnerUserID {
		return "owner"
	}
	permission := "none"
	if s.Visibility == "public" {
		permission = "public"
	}
	if s.Visibility == "private" {
		return permission
	}
	for _, g := range s.Grants {
		for _, id := range teams {
			if id == g.TeamID {
				if g.Permission == "editor" {
					return "editor"
				}
				permission = "viewer"
			}
		}
	}
	return permission
}

func ValidateGit(source GitSource) error {
	u, err := url.Parse(source.URL)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(source.URL) > 2048 {
		return ErrInvalid
	}
	if len(source.Branch) > 200 || strings.HasPrefix(source.Branch, "-") || strings.ContainsAny(source.Branch, "\x00\r\n") {
		return ErrInvalid
	}
	if source.Subpath != "" && source.Subpath != "." {
		if path.IsAbs(source.Subpath) || len(source.Subpath) > 1024 || strings.ContainsAny(source.Subpath, "\\\x00") {
			return ErrInvalid
		}
		for _, part := range strings.Split(source.Subpath, "/") {
			if part == "" || part == "." || part == ".." {
				return ErrInvalid
			}
		}
	}
	return nil
}
