package rest

import (
	"context"
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	coredomain "agentguild.dev/agentguild/backend/internal/domain"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	"github.com/go-chi/chi/v5"
)

// AgentReputationService 是声望 v2 的只读服务边界。它与 v1 的
// ReputationService 并存：v1 服务 /v1/reputation，两者互不影响。
type AgentReputationService interface {
	GetAgentReputation(ctx context.Context, agentID string) (reputationapp.Envelope[reputationapp.AgentReputationView], error)
	GetSelfReputation(ctx context.Context, principal auth.Principal) (reputationapp.Envelope[reputationapp.AgentReputationView], error)
}

// ReputationRebuilder 触发全量重算。evaluatedAt 由调用方显式给出，
// 缺省时用服务端当前时刻，并原样回报到响应里——否则结果不可复现。
type ReputationRebuilder interface {
	Rebuild(ctx context.Context, algorithmVersion string, evaluatedAt time.Time) (reputationapp.RebuildResult, error)
}

// WithAgentReputationService 挂载声望 v2 只读 API。
func WithAgentReputationService(svc AgentReputationService) Option {
	return func(s *Server) { s.agentReputationSvc = svc }
}

// WithReputationRebuilder 挂载管理员全量重算入口。
func WithReputationRebuilder(rebuilder ReputationRebuilder, algorithmVersion string) Option {
	return func(s *Server) {
		s.reputationRebuilder = rebuilder
		s.reputationAlgorithmVersion = algorithmVersion
	}
}

// getPublicAgentReputation 是匿名可读的声望卡片。视图里没有任何 sponsor
// 租户标识，因此可以安全地公开。
func (s *Server) getPublicAgentReputation(w http.ResponseWriter, r *http.Request) {
	if s.agentReputationSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "reputation v2 is not configured")
		return
	}
	result, err := s.agentReputationSvc.GetAgentReputation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, auth.Principal{})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getSelfReputation(w http.ResponseWriter, r *http.Request) {
	if s.agentReputationSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "reputation v2 is not configured")
		return
	}
	principal := mustPrincipal(r)
	result, err := s.agentReputationSvc.GetSelfReputation(r.Context(), principal)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// rebuildReputation 触发全量重算。它是管理动作，只接受管理员主体。
func (s *Server) rebuildReputation(w http.ResponseWriter, r *http.Request) {
	if s.reputationRebuilder == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "reputation v2 is not configured")
		return
	}
	principal := mustPrincipal(r)
	if !principal.IsAdmin && !isAdministrator(principal) {
		mapDomainError(w, coredomain.ErrForbidden, principal)
		return
	}

	var body struct {
		AlgorithmVersion string    `json:"algorithm_version"`
		EvaluatedAt      time.Time `json:"evaluated_at"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	algorithmVersion := body.AlgorithmVersion
	if algorithmVersion == "" {
		algorithmVersion = s.reputationAlgorithmVersion
	}
	evaluatedAt := body.EvaluatedAt
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}

	result, err := s.reputationRebuilder.Rebuild(r.Context(), algorithmVersion, evaluatedAt)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, reputationapp.Envelope[reputationapp.RebuildResult]{
		Data: result,
		Meta: reputationapp.Meta{ServerTime: time.Now().UTC()},
	})
}
