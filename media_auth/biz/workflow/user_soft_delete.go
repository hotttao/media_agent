package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainauth "media_agent/media_auth/biz/domain/auth"
	"media_agent/media_auth/biz/shared/ids"
)

var ErrLastActiveAdmin = errors.New("cannot delete the last active admin")

type TransactionRunner interface {
	RunInTx(context.Context, func(context.Context) error) error
}

type softDeleteUsers interface {
	FindByID(context.Context, string) (domainauth.User, error)
	CountActiveAdmins(context.Context) (int, error)
	SoftDeleteUser(context.Context, string, string) (domainauth.User, error)
}
type userTokenRevoker interface {
	RevokeAllForUser(context.Context, string) error
}
type userOAuthRevoker interface {
	RevokeUserCredentials(context.Context, string) error
}
type userMCPRevoker interface {
	DeleteTokensForUser(context.Context, string) error
}

// UserSoftDelete 原子提交用户终态与访问撤销事件，并异步清理各认证域凭据。
type UserSoftDelete struct {
	users  softDeleteUsers
	outbox domainauth.AccessRevocationOutbox
	tokens userTokenRevoker
	oauth  userOAuthRevoker
	mcp    userMCPRevoker
	tx     TransactionRunner
}

func NewUserSoftDelete(users softDeleteUsers, outbox domainauth.AccessRevocationOutbox,
	tokens userTokenRevoker, oauth userOAuthRevoker, mcp userMCPRevoker,
	tx TransactionRunner) *UserSoftDelete {
	return &UserSoftDelete{users: users, outbox: outbox, tokens: tokens, oauth: oauth, mcp: mcp, tx: tx}
}

func (w *UserSoftDelete) Delete(ctx context.Context, currentUserID, targetID string) error {
	if currentUserID == targetID {
		return errors.New("cannot delete yourself")
	}
	if w.tx == nil {
		return errors.New("transaction runner is required")
	}
	return w.tx.RunInTx(ctx, func(txCtx context.Context) error {
		target, err := w.users.FindByID(txCtx, targetID)
		if err != nil || !target.IsActive() {
			if err != nil {
				return err
			}
			return domainauth.ErrUserNotFound
		}
		if target.Role == "admin" {
			count, err := w.users.CountActiveAdmins(txCtx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastActiveAdmin
			}
		}
		deleted, err := w.users.SoftDeleteUser(txCtx, targetID, currentUserID)
		if err != nil {
			return err
		}
		return w.outbox.Enqueue(txCtx, domainauth.AccessRevocationEvent{
			ID: ids.New(), UserID: deleted.ID, AuthVersion: deleted.AuthVersion,
		})
	})
}

// ProcessPending 执行一批可靠撤销。各操作幂等，整批单事件置于事务中。
func (w *UserSoftDelete) ProcessPending(ctx context.Context) error {
	events, err := w.outbox.ClaimPending(ctx, 20)
	if err != nil {
		return err
	}
	for _, event := range events {
		err := w.tx.RunInTx(ctx, func(txCtx context.Context) error {
			if err := w.tokens.RevokeAllForUser(txCtx, event.UserID); err != nil {
				return err
			}
			if err := w.oauth.RevokeUserCredentials(txCtx, event.UserID); err != nil {
				return err
			}
			if err := w.mcp.DeleteTokensForUser(txCtx, event.UserID); err != nil {
				return err
			}
			return w.outbox.MarkDelivered(txCtx, event.ID)
		})
		if err != nil {
			if markErr := w.outbox.MarkFailed(ctx, event.ID, err.Error()); markErr != nil {
				return fmt.Errorf("revoke user %s: %v; mark failed: %w", event.UserID, err, markErr)
			}
		}
	}
	return nil
}

func (w *UserSoftDelete) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := w.ProcessPending(ctx); err != nil { /* retry on next tick */
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
