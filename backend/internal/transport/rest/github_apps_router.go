package rest

import (
	"errors"
	"net/http"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/go-chi/chi/v5"
)

type githubAppItemsView struct {
	Items []gitapp.GitHubAppView `json:"items"`
}

func (s *Server) listGitHubApps(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	items, err := s.gitHubAppManager.List(r.Context(), principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, githubAppItemsView{Items: items})
}

func (s *Server) getGitHubAppByID(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	view, err := s.gitHubAppManager.GetByID(r.Context(), principal.TenantID, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, view)
}

func (s *Server) deleteGitHubAppByID(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	if err := s.gitHubAppManager.DeleteByID(r.Context(), principal.TenantID, chi.URLParam(r, "id")); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) testGitHubAppByID(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	source, err := s.gitHubAppManager.IssueSourceForApp(r.Context(), principal.TenantID, chi.URLParam(r, "id"))
	if errors.Is(err, git.ErrGitHubAppNotConfigured) {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, testGitHubConnection(r, source, err))
}
