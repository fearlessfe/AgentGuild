package rest

import (
	"net/http"

	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaldomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/go-chi/chi/v5"
)

func (s *Server) listBenchmarks(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.evaluations.ListBenchmarkSetSummaries(r.Context(), principal, principal.TenantID)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createBenchmark(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tasks       []string `json:"tasks"`
		IsActive    bool     `json:"is_active"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	tasks := make([]evaldomain.BenchmarkTask, 0, len(body.Tasks))
	for i, ref := range body.Tasks {
		tasks = append(tasks, evaldomain.BenchmarkTask{TaskRef: ref, Ordering: i})
	}

	result, err := s.evaluations.CreateBenchmarkSet(r.Context(), evaluationapp.CreateBenchmarkSet{
		TenantID:    principal.TenantID,
		Name:        body.Name,
		Description: body.Description,
		Tasks:       tasks,
		IsActive:    body.IsActive,
		CreatedBy:   principal.OwnerID,
		IsAdmin:     principal.IsAdmin,
	})
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"benchmark_set_id": result.BenchmarkSet.ID(),
			"version_number":   result.BenchmarkSet.VersionNumber(),
		},
	})
}

func (s *Server) getBenchmark(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.evaluations.GetBenchmarkSetSummary(r.Context(), principal, principal.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	agentVersionID := r.URL.Query().Get("agent_version_id")
	result, err := s.evaluations.ListEvaluationRunSummaries(r.Context(), principal, principal.TenantID, agentVersionID)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getEvaluation(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.evaluations.GetEvaluationRunDetail(r.Context(), principal, principal.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}
