package application_access

import (
	"context"
	"media_agent/packages/platform-go-sdk/biz/shared/contract"
)

func (u *UserClient) Me(ctx context.Context) (contract.UserProfile, error) {
	var zero contract.UserProfile
	token, e := u.token(ctx)
	if e != nil {
		return zero, e
	}
	v, e := u.api.Me(ctx, "Bearer "+token)
	u.invalidate(e)
	return v, e
}
