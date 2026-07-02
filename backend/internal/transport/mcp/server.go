package mcp

import (
	"context"
	"net/http"
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

// Server 暴露任务生命周期的 MCP 工具。
type Server struct {
	svc      applicationService
	verifier auth.TokenVerifier
}

// NewServer 创建 MCP server；svc 通常是 *application.Service。
func NewServer(svc applicationService, verifier auth.TokenVerifier) *Server {
	return &Server{svc: svc, verifier: verifier}
}

// Handler 返回无状态 Streamable HTTP MCP handler，路径通常为 /mcp。
func (s *Server) Handler() http.Handler {
	base := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return s.mcpServer(r)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return s.authenticate(limitRequestBody(base, 1<<20))
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
