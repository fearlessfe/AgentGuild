package auth

import "context"

// principalContextKey 用于在 context 中存放已认证的 Principal；未导出以避免外部误用。
type principalContextKey struct{}

// WithPrincipal 把 Principal 注入 context，供 transport 层下游 handler 读取。
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalFrom 从 context 读取 Principal；第二个返回值表示是否成功读取。
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalContextKey{}).(Principal)
	return p, ok
}
