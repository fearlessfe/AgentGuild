package rest

import (
	"net/http"

	agentversionapp "agentguild.dev/agentguild/backend/internal/agentversion/application"
	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	"github.com/go-chi/chi/v5"
)

func (s *Server) listAgentVersions(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	agentID := chi.URLParam(r, "id")
	result, err := s.versions.ListVersions(r.Context(), principal.TenantID, agentID)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createAgentVersion(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Runtime               string   `json:"runtime"`
		Model                 string   `json:"model"`
		Capabilities          []string `json:"capabilities"`
		PromptRef             string   `json:"prompt_ref"`
		SkillRefs             []string `json:"skill_refs"`
		MemoryRef             string   `json:"memory_ref"`
		ToolRefs              []string `json:"tool_refs"`
		EnvironmentDigest     string   `json:"environment_digest"`
		ApprovedExperienceIDs []string `json:"approved_experience_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := s.versions.CreateDraft(r.Context(), agentversionapp.CreateDraft{
		TenantID:              principal.TenantID,
		AgentID:               chi.URLParam(r, "id"),
		CreatedBy:             principal.OwnerID,
		IsAdmin:               principal.IsAdmin,
		Runtime:               body.Runtime,
		Model:                 body.Model,
		Capabilities:          body.Capabilities,
		PromptRef:             body.PromptRef,
		SkillRefs:             body.SkillRefs,
		MemoryRef:             body.MemoryRef,
		ToolRefs:              body.ToolRefs,
		EnvironmentDigest:     body.EnvironmentDigest,
		ApprovedExperienceIDs: body.ApprovedExperienceIDs,
	})
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"version_id":     result.Version.ID,
			"version_number": result.Version.VersionNumber,
			"status":         result.Version.Status,
		},
		"meta": map[string]any{
			"server_time": result.Version.CreatedAt,
		},
	})
}

func (s *Server) getAgentVersion(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	result, err := s.versions.GetVersion(r.Context(), principal.TenantID, chi.URLParam(r, "id"), chi.URLParam(r, "version_id"))
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) diffAgentVersion(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		BaseVersionID string `json:"base_version_id,omitempty"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := s.versions.GetVersionDiff(r.Context(), principal.TenantID, chi.URLParam(r, "id"), chi.URLParam(r, "version_id"), body.BaseVersionID)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) startAgentVersionEvaluation(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		BenchmarkSetID     string `json:"benchmark_set_id"`
		EnvironmentDigest  string `json:"environment_digest"`
		ScoringRuleVersion string `json:"scoring_rule_version,omitempty"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := s.evaluations.StartEvaluationRun(r.Context(), evaluationapp.StartEvaluationRun{
		TenantID:           principal.TenantID,
		AgentID:            chi.URLParam(r, "id"),
		VersionID:          chi.URLParam(r, "version_id"),
		BenchmarkSetID:     body.BenchmarkSetID,
		EnvironmentDigest:  body.EnvironmentDigest,
		ScoringRuleVersion: body.ScoringRuleVersion,
		ActorID:            principal.OwnerID,
		IsAdmin:            principal.IsAdmin,
	})
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"evaluation_run_id": result.EvaluationRun.ID(),
			"status":            result.EvaluationRun.Status(),
		},
	})
}

func (s *Server) promoteAgentVersion(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	if err := s.versions.Promote(r.Context(), agentversionapp.Promote{
		TenantID:  principal.TenantID,
		AgentID:   chi.URLParam(r, "id"),
		VersionID: chi.URLParam(r, "version_id"),
		ActorID:   principal.OwnerID,
		IsAdmin:   principal.IsAdmin,
	}); err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"promoted": true}})
}

func (s *Server) rollbackAgentVersion(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	if err := s.versions.Rollback(r.Context(), agentversionapp.Rollback{
		TenantID:  principal.TenantID,
		AgentID:   chi.URLParam(r, "id"),
		VersionID: chi.URLParam(r, "version_id"),
		ActorID:   principal.OwnerID,
		IsAdmin:   principal.IsAdmin,
	}); err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"rolled_back": true}})
}
