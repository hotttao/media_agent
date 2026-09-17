package organization

import (
	"context"
	"errors"
	"time"
)

type Organization struct {
	ID, Name, Status string
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Membership struct {
	OrganizationID, UserID, UserName, Role string
	JoinedAt                               time.Time
}

type Summary struct {
	Organization
	MyRole string
}

type Repository interface {
	ListForUser(context.Context, string) ([]Summary, error)
	Create(context.Context, string, string) (Summary, error)
	Get(context.Context, string) (Organization, error)
	Update(context.Context, string, *string, *string) (Organization, error)
	GetMembership(context.Context, string, string) (Membership, error)
	ListMembers(context.Context, string) ([]Membership, error)
	PutMember(context.Context, string, string, string) (Membership, error)
	RemoveMember(context.Context, string, string) error
	CountAdmins(context.Context, string) (int, error)
	UserExists(context.Context, string) (bool, error)
	GetEntitlements(context.Context, string) ([]string, error)
	SetEntitlements(context.Context, string, string, []string) ([]string, error)
}

var (
	ErrNotFound  = errors.New("organization not found")
	ErrNotMember = errors.New("organization membership not found")
)
