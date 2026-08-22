package afs_binding

import (
	"context"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("afs binding not found")

type Binding struct {
	ID, WorkspaceID, ExternalRef, DesiredState, ObservedState string
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
}
type Member struct {
	BindingID, SubjectID, SubjectType, ExternalMemberRef, Status string
	UpdatedAt                                                    time.Time
}
type Repository interface {
	Create(context.Context, *Binding) error
	ListOwned(context.Context, string, string) ([]Binding, error)
	GetOwned(context.Context, string, string) (*Binding, error)
	Delete(context.Context, *Binding) error
	Reconcile(context.Context, *Binding, string) error
	ListMembers(context.Context, string) ([]Member, error)
	AddMember(context.Context, *Member) error
	RemoveMember(context.Context, string, string) error
}

func Validate(ref, desired string) (string, string, error) {
	ref, desired = strings.TrimSpace(ref), strings.TrimSpace(desired)
	if ref == "" {
		return "", "", errors.New("external_ref is required")
	}
	if desired == "" {
		desired = "active"
	}
	if desired != "active" && desired != "inactive" {
		return "", "", errors.New("desired_state must be active or inactive")
	}
	return ref, desired, nil
}
func ValidateMember(id, typ, external string) (string, string, string, error) {
	id, typ, external = strings.TrimSpace(id), strings.TrimSpace(typ), strings.TrimSpace(external)
	if id == "" || (typ != "user" && typ != "team") {
		return "", "", "", errors.New("subject_id and subject_type(user/team) are required")
	}
	return id, typ, external, nil
}
