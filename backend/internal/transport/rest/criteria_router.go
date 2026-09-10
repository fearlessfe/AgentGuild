package rest

import (
	"context"
	"net/http"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	"github.com/go-chi/chi/v5"
)

// CriteriaService 暴露验收标准的逐条验证证据。
type CriteriaService interface {
	ExecutionCriteria(ctx context.Context, principal auth.Principal, executionID string) (application.Envelope[contributionapp.CriterionCoverageView], error)
}

// WithCriteriaService 挂载验收证据 REST API。
func WithCriteriaService(svc CriteriaService) Option {
	return func(s *Server) { s.criteriaSvc = svc }
}

func (s *Server) getExecutionCriteria(w http.ResponseWriter, r *http.Request) {
	if s.criteriaSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "criteria service is not configured")
		return
	}
	principal := mustPrincipal(r)
	result, err := s.criteriaSvc.ExecutionCriteria(r.Context(), principal, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
