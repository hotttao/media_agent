package shared

import "context"

// callerKey 保留向后兼容（旧代码可能直接用 shared.CallerFromContext）。
type callerKey struct{}

func WithCaller(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, callerKey{}, userID)
}

// CallerFromContext 从 ctx 取 caller user_id（向后兼容；新代码用 auth.CallerID）。
func CallerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(callerKey{}).(string)
	return v
}
