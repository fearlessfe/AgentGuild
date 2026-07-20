package rest

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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
	writeEnvelope(w, http.StatusOK, evaluationapp.BenchmarkSetPage{Items: result})
}

func (s *Server) createBenchmark(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Tasks       []json.RawMessage `json:"tasks"`
		IsActive    bool              `json:"is_active"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	// Each task entry is either a bare task reference string (legacy form) or
	// an object carrying the full task definition for real executors.
	tasks := make([]evaldomain.BenchmarkTask, 0, len(body.Tasks))
	for i, raw := range body.Tasks {
		var task struct {
			TaskRef      string   `json:"task_ref"`
			Title        string   `json:"title"`
			Problem      string   `json:"problem"`
			Constraints  []string `json:"constraints"`
			Requirements []string `json:"requirements"`
			IsSecurity   bool     `json:"is_security"`
		}
		var ref string
		if err := json.Unmarshal(raw, &ref); err == nil {
			task.TaskRef = ref
		} else if err := json.Unmarshal(raw, &task); err != nil {
			writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "tasks must contain task reference strings or task definition objects", "tasks")
			return
		}
		task.TaskRef = strings.TrimSpace(task.TaskRef)
		tasks = append(tasks, evaldomain.BenchmarkTask{
			TaskRef:      task.TaskRef,
			Ordering:     i,
			Title:        task.Title,
			Problem:      task.Problem,
			Constraints:  task.Constraints,
			Requirements: task.Requirements,
			IsSecurity:   task.IsSecurity,
		})
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
		"meta": map[string]any{
			"server_time": time.Now(),
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
	writeEnvelope(w, http.StatusOK, *result)
}

func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	agentVersionID := r.URL.Query().Get("agent_version_id")
	result, err := s.evaluations.ListEvaluationRunDetails(r.Context(), principal, principal.TenantID, agentVersionID)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeEnvelope(w, http.StatusOK, evaluationapp.EvaluationRunPage{Items: result})
}

func (s *Server) getEvaluation(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.evaluations.GetEvaluationRunDetail(r.Context(), principal, principal.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeEnvelope(w, http.StatusOK, *result)
}
