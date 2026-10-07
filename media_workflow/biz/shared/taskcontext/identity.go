// Package taskcontext carries read-only transport identity to an internal task
// function. It does not decide execution state, routing, retries or successors.
package taskcontext

import "context"

type Identity struct {
	WorkflowID, TaskID, TaskType, Reference string
}

type identityKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok && identity.WorkflowID != "" && identity.TaskID != "" && identity.TaskType != ""
}
