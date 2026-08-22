package environment_grant

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("environment grant not found")
	ErrRevoked  = errors.New("environment grant is revoked")
)

type Grant struct {
	ID, EnvironmentID, SubjectID, SubjectType, Permission, Status string
	CreatedAt, RevokedAt                                          *time.Time
}
type Repository interface {
	Create(context.Context, *Grant) error
	List(context.Context, string) ([]Grant, error)
	Get(context.Context, string, string) (*Grant, error)
	Revoke(context.Context, *Grant) error
	EnvironmentExists(context.Context, string) bool
}

func Validate(subject, typ, perm string) (string, string, string, error) {
	subject, typ, perm = strings.TrimSpace(subject), strings.TrimSpace(typ), strings.TrimSpace(perm)
	if subject == "" || (typ != "user" && typ != "team") || (perm != "use" && perm != "admin") {
		return "", "", "", errors.New("subject_id, subject_type(user/team), permission(use/admin) are required")
	}
	return subject, typ, perm, nil
}
