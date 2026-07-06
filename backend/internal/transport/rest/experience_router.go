package rest

import (
	"net/http"
	"time"

	agentexperienceapp "agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"github.com/go-chi/chi/v5"
)

func (s *Server) listAgentExperiences(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	status := r.URL.Query().Get("status")
	result, err := s.experiences.ListCandidates(r.Context(), principal.TenantID, chi.URLParam(r, "id"), status)
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeEnvelope(w, http.StatusOK, agentexperienceapp.ExperienceCandidatePage{Items: result})
}

func (s *Server) createAgentExperience(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		SubmissionID  string `json:"submission_id"`
		EvidenceBytes []byte `json:"evidence_bytes"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	result, err := s.experiences.ExtractCandidate(r.Context(), agentexperienceapp.ExtractCandidate{
		TenantID:      principal.TenantID,
		AgentID:       chi.URLParam(r, "id"),
		SubmissionID:  body.SubmissionID,
		EvidenceBytes: body.EvidenceBytes,
		CreatedBy:     principal.OwnerID,
		IsAdmin:       principal.IsAdmin,
	})
	if err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"experience_id":     result.Candidate.ID,
			"status":            result.Candidate.Status,
			"sensitivity_class": result.Candidate.SensitivityClass,
		},
		"meta": map[string]any{
			"server_time": time.Now(),
		},
	})
}

func (s *Server) approveAgentExperience(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	if err := s.experiences.ReviewCandidate(r.Context(), agentexperienceapp.ReviewCandidate{
		TenantID:    principal.TenantID,
		AgentID:     chi.URLParam(r, "id"),
		CandidateID: chi.URLParam(r, "experience_id"),
		Action:      "approve",
		ReviewerID:  principal.OwnerID,
		IsAdmin:     principal.IsAdmin,
	}); err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"approved": true},
		"meta": map[string]any{"server_time": time.Now()},
	})
}

func (s *Server) rejectAgentExperience(w http.ResponseWriter, r *http.Request) {
	principal := identityPrincipalFromAuth(mustPrincipal(r))
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.experiences.ReviewCandidate(r.Context(), agentexperienceapp.ReviewCandidate{
		TenantID:    principal.TenantID,
		AgentID:     chi.URLParam(r, "id"),
		CandidateID: chi.URLParam(r, "experience_id"),
		Action:      "reject",
		Reason:      body.Reason,
		ReviewerID:  principal.OwnerID,
		IsAdmin:     principal.IsAdmin,
	}); err != nil {
		mapDomainError(w, err, mustPrincipal(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"rejected": true},
		"meta": map[string]any{"server_time": time.Now()},
	})
}
