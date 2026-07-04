package shared

import "context"

type callerKey struct{}

func WithCaller(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, callerKey{}, userID)
}

func CallerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(callerKey{}).(string)
	return v
}
