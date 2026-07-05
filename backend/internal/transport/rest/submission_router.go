package rest

import (
	"encoding/json"
	"net/http"
	"strings"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/go-chi/chi/v5"
)

func (s *Server) createSubmission(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		RequestID     string          `json:"request_id"`
		Repo          string          `json:"repo"`
		Branch        string          `json:"branch"`
		CommitSHA     string          `json:"commit_sha"`
		BaseCommitSHA string          `json:"base_commit_sha"`
		Summary       string          `json:"summary"`
		Tests         *string         `json:"tests,omitempty"`
		Evidence      json.RawMessage `json:"evidence,omitempty"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	executionID := chi.URLParam(r, "id")
	execResult, err := s.svc.GetExecution(r.Context(), principal, application.GetExecution{ExecutionID: executionID})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}

	allowed, forbidden := parsePathConstraints(execResult.Data.TaskConstraints)
	result, err := s.submissions.CreateSubmission(r.Context(), gitPrincipal(principal), gitapp.CreateSubmission{
		RequestID:      idempotencyKey,
		ExecutionID:    executionID,
		TaskID:         execResult.Data.TaskID,
		Repo:           body.Repo,
		Branch:         body.Branch,
		CommitSHA:      body.CommitSHA,
		BaseCommitSHA:  body.BaseCommitSHA,
		Summary:        body.Summary,
		Tests:          body.Tests,
		Evidence:       []byte(body.Evidence),
		AllowedPaths:   allowed,
		ForbiddenPaths: forbidden,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getSubmission(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	submissionID := chi.URLParam(r, "id")

	result, err := s.submissions.GetSubmission(r.Context(), gitPrincipal(principal), gitapp.GetSubmission{SubmissionID: submissionID})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}

	// Enforce that the caller can access the owning execution. GetExecution
	// applies the owner/admin policy and returns a secure not_found otherwise.
	_, err = s.svc.GetExecution(r.Context(), principal, application.GetExecution{ExecutionID: result.Data.ExecutionID})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func gitPrincipal(p auth.Principal) gitapp.Principal {
	return gitapp.Principal{
		TenantID:       p.TenantID,
		OwnerID:        p.OwnerID,
		OwnerEmail:     p.OwnerEmail,
		IsAdmin:        p.IsAdmin,
		AgentID:        p.AgentID,
		AgentVersionID: p.AgentVersionID,
		Scopes:         p.Scopes,
		RepoScope:      p.RepoScope,
	}
}

func parsePathConstraints(constraints []string) (allowed, forbidden []string) {
	for _, c := range constraints {
		if strings.HasPrefix(c, "path:allowed:") {
			allowed = append(allowed, strings.TrimPrefix(c, "path:allowed:"))
			continue
		}
		if strings.HasPrefix(c, "path:forbidden:") {
			forbidden = append(forbidden, strings.TrimPrefix(c, "path:forbidden:"))
		}
	}
	return
}
