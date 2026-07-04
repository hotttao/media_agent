package shared

import "context"

// callerKey context key for caller user_id（由 auth 中间件注入）。
type callerKey struct{}

// WithCaller 把 caller user_id 注入 context。
func WithCaller(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, callerKey{}, userID)
}

// CallerFromContext 取 caller user_id；未注入返回 ""。
func CallerFromContext(ctx context.Context) string {
	v, _ := ctx.Value(callerKey{}).(string)
	return v
}
