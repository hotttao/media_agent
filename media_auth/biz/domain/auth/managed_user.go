package auth

import (
	"context"
	"errors"
)

var ErrManagedUser = errors.New("application-managed user cannot use this identity operation")

type ManagedUserRepo interface {
	CreateManaged(context.Context, string, string, string) (User, error)
	LockManaged(context.Context, string) (User, error)
	UpdateManagedProfile(context.Context, string, string, string) error
}
