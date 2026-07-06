package rest

import (
	"net/http"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

func (s *Server) getGitHubApp(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	view, err := s.gitHubAppManager.Get(r.Context(), principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) upsertGitHubApp(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	var body struct {
		AppID          int64  `json:"app_id"`
		InstallationID int64  `json:"installation_id"`
		PrivateKey     string `json:"private_key"`
		BaseURL        string `json:"base_url"`
		Provider       string `json:"provider"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.gitHubAppManager.Upsert(r.Context(), gitapp.UpsertGitHubApp{
		TenantID:       principal.TenantID,
		Provider:       body.Provider,
		AppID:          body.AppID,
		InstallationID: body.InstallationID,
		PrivateKey:     body.PrivateKey,
		BaseURL:        body.BaseURL,
	}); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	view, err := s.gitHubAppManager.Get(r.Context(), principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) deleteGitHubApp(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if err := s.gitHubAppManager.Delete(r.Context(), principal.TenantID); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}
