package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// applicationService 是 REST 层消费的应用服务边界；*application.Service 天然满足此接口。
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

// Server 暴露任务生命周期的 REST API。
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

// NewServer 创建 REST server；svc 通常是 *application.Service。
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

// Router 返回 chi 路由；调用方负责监听与优雅关闭。
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(jsonResponse)
	r.Use(s.authenticate)
	r.Use(s.rateLimit)

	r.Route("/v1", func(r chi.Router) {
		r.Post("/tasks", s.publishTask)
		r.Get("/tasks", s.listTasks)
		r.Get("/tasks/{id}", s.getTask)
		r.Post("/tasks/{id}:claim", s.claimTask)
		r.Post("/tasks/{id}:cancel", s.cancelTask)

		r.Get("/executions/{id}", s.getExecution)
		r.Post("/executions/{id}:start", s.startExecution)
		r.Post("/executions/{id}:heartbeat", s.heartbeatExecution)
	})
	return r
}

func jsonResponse(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := mustPrincipal(r)
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
		next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
	})
}

func (s *Server) publishTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID    string    `json:"request_id"`
		Type         string    `json:"type"`
		Title        string    `json:"title"`
		Problem      string    `json:"problem"`
		Constraints  []string  `json:"constraints"`
		Requirements []string  `json:"requirements"`
		Deadline     time.Time `json:"deadline"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.PublishTask(r.Context(), principal, application.PublishTask{
		RequestID:    idempotencyKey,
		Type:         body.Type,
		Title:        body.Title,
		Problem:      body.Problem,
		Constraints:  body.Constraints,
		Requirements: body.Requirements,
		Deadline:     body.Deadline,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limit is invalid", "limit")
		return
	}
	query := application.ListTasks{
		Limit:                   limit,
		Cursor:                  r.URL.Query().Get("cursor"),
		PublisherAgentVersionID: r.URL.Query().Get("publisher_agent_version_id"),
	}
	if statuses := r.URL.Query()["status"]; len(statuses) > 0 {
		query.Statuses = make([]domain.TaskStatus, 0, len(statuses))
		for _, s := range statuses {
			query.Statuses = append(query.Statuses, domain.TaskStatus(s))
		}
	}

	result, err := s.svc.ListTasks(r.Context(), principal, query)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	result, err := s.svc.GetTask(r.Context(), principal, application.GetTask{TaskID: chi.URLParam(r, "id")})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) claimTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.ClaimTask(r.Context(), principal, application.ClaimTask{
		RequestID: idempotencyKey,
		TaskID:    chi.URLParam(r, "id"),
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
		Reason    string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.CancelTask(r.Context(), principal, application.CancelTask{
		RequestID: idempotencyKey,
		TaskID:    chi.URLParam(r, "id"),
		Reason:    body.Reason,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	result, err := s.svc.GetExecution(r.Context(), principal, application.GetExecution{ExecutionID: chi.URLParam(r, "id")})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) startExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID       string `json:"request_id"`
		LeaseGeneration int64  `json:"lease_generation"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.StartExecution(r.Context(), principal, application.StartExecution{
		RequestID:       idempotencyKey,
		ExecutionID:     chi.URLParam(r, "id"),
		LeaseGeneration: body.LeaseGeneration,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) heartbeatExecution(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID       string `json:"request_id"`
		LeaseGeneration int64  `json:"lease_generation"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.svc.HeartbeatExecution(r.Context(), principal, application.HeartbeatExecution{
		RequestID:       idempotencyKey,
		ExecutionID:     chi.URLParam(r, "id"),
		LeaseGeneration: body.LeaseGeneration,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// resolveIdempotencyKey 优先使用 Idempotency-Key header，其次使用 body 中的 request_id。
// REST 规范以 header 为首选，保留 body 字段用于兼容测试与旧客户端。
func resolveIdempotencyKey(r *http.Request, bodyRequestID string, w http.ResponseWriter) (string, bool) {
	if header := r.Header.Get("Idempotency-Key"); header != "" {
		return header, true
	}
	if bodyRequestID != "" {
		return bodyRequestID, true
	}
	writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "idempotency_key is required", "idempotency_key")
	return "", false
}

func parseLimit(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func mustPrincipal(r *http.Request) auth.Principal {
	p, _ := principalFrom(r.Context())
	return p
}

type principalKey struct{}

func withPrincipal(ctx context.Context, p auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func principalFrom(ctx context.Context) (auth.Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(auth.Principal)
	return p, ok
}
