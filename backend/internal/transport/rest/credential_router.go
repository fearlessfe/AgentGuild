package rest

import (
	"net/http"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/go-chi/chi/v5"
)

func (s *Server) issueCredential(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	executionID := chi.URLParam(r, "id")

	var body struct {
		RequestID  string `json:"request_id"`
		Repo       string `json:"repo"`
		Branch     string `json:"branch,omitempty"`
		BaseCommit string `json:"base_commit"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	idempotencyKey, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}

	result, err := s.credentials.IssueCredential(r.Context(), gitPrincipal(principal), gitapp.IssueCredential{
		RequestID:   idempotencyKey,
		ExecutionID: executionID,
		Repo:        body.Repo,
		Branch:      body.Branch,
		BaseCommit:  body.BaseCommit,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getCredential(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	executionID := chi.URLParam(r, "id")

	result, err := s.credentials.GetCredential(r.Context(), gitPrincipal(principal), gitapp.GetCredential{ExecutionID: executionID})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) revokeCredential(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	executionID := chi.URLParam(r, "id")

	result, err := s.credentials.RevokeCredential(r.Context(), gitPrincipal(principal), gitapp.RevokeCredential{ExecutionID: executionID})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
