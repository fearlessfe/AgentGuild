package mcp

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// applicationService 是 MCP 层消费的应用服务边界；*application.Service 天然满足此接口。
type applicationService interface {
	PublishTask(ctx context.Context, principal auth.Principal, command application.PublishTask) (application.Envelope[application.TaskView], error)
	ListTasks(ctx context.Context, principal auth.Principal, query application.ListTasks) (application.Envelope[[]application.TaskView], error)
	GetTask(ctx context.Context, principal auth.Principal, query application.GetTask) (application.Envelope[application.TaskView], error)
	CancelTask(ctx context.Context, principal auth.Principal, command application.CancelTask) (application.Envelope[application.TaskView], error)
	ClaimTask(ctx context.Context, principal auth.Principal, command application.ClaimTask) (application.Envelope[application.ExecutionView], error)
	StartExecution(ctx context.Context, principal auth.Principal, command application.StartExecution) (application.Envelope[application.ExecutionView], error)
	HeartbeatExecution(ctx context.Context, principal auth.Principal, command application.HeartbeatExecution) (application.Envelope[application.ExecutionView], error)
	GetExecution(ctx context.Context, principal auth.Principal, query application.GetExecution) (application.Envelope[application.ExecutionView], error)
}

// RateLimiter 决定请求是否被限流；若不允许，返回建议等待秒数。
type RateLimiter interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfter int)
}

type noopRateLimiter struct{}

func (noopRateLimiter) Allow(context.Context, string) (bool, int) { return true, 0 }

// Server 暴露任务生命周期的 MCP 工具。
type Server struct {
	svc      applicationService
	verifier auth.TokenVerifier
	limiter  RateLimiter
}

// Option 配置 Server。
type Option func(*Server)

// WithRateLimiter 替换默认的无限流实现。
func WithRateLimiter(l RateLimiter) Option {
	return func(s *Server) { s.limiter = l }
}

// NewServer 创建 MCP server；svc 通常是 *application.Service。
func NewServer(svc applicationService, verifier auth.TokenVerifier, opts ...Option) *Server {
	s := &Server{
		svc:      svc,
		verifier: verifier,
		limiter:  noopRateLimiter{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Handler 返回无状态 Streamable HTTP MCP handler，路径通常为 /mcp。
func (s *Server) Handler() http.Handler {
	base := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer(r)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return s.authenticate(s.rateLimit(limitRequestBody(base, 1<<20)))
}

// limitRequestBody 限制请求体大小，Content-Length 超过上限直接返回 413；
// 对分块传输由 http.MaxBytesReader 兜底。
func limitRequestBody(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

// mcpServer 为每个 HTTP 请求构造一个带工具的 MCP Server。
// 无状态模式下每次请求都会重新构造；Principal 已由 authenticate 注入请求 context。
func (s *Server) mcpServer(r *http.Request) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "agentguild-task-lifecycle", Version: "1.0.0"},
		nil,
	)
	principal, _ := auth.PrincipalFrom(r.Context())
	registerTools(server, s.svc, principal)
	return server
}

// authenticate 从 HTTP Authorization Header 解析 OAuth Principal 并写入请求 context。
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid authorization")
			return
		}
		principal, err := s.verifier.Verify(r.Context(), parts[1])
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token verification failed")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

// rateLimit 在认证之后根据 Principal 与 HTTP 方法+路径进行限流。
// 若未设置 limiter 或允许通过，则继续；否则返回 429。
func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Method + "|" + r.URL.Path
		if principal.TenantID != "" {
			key = principal.TenantID + "|" + key
		}
		allowed, retryAfter := s.limiter.Allow(r.Context(), key)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
