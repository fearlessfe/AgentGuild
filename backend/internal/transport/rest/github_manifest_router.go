package rest

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"

	"agentguild.dev/agentguild/backend/internal/git"
)

// githubManifest builds a GitHub App manifest + signed state for the current
// tenant and returns an auto-submitting HTML form that POSTs the manifest to
// GitHub's apps/new page.
func (s *Server) githubManifest(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	manifest, state, redirectURL, err := s.manifest.BuildManifest(principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}

	action := fmt.Sprintf("%s?state=%s", redirectURL, url.QueryEscape(state))
	page := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>Connecting to GitHub…</title></head>
<body onload="document.forms[0].submit()">
<form action="%s" method="post">
<input type="hidden" name="manifest" value="%s">
<input type="hidden" name="state" value="%s">
<noscript><button type="submit">Continue to GitHub</button></noscript>
</form>
</body>
</html>`, html.EscapeString(action), html.EscapeString(manifest), html.EscapeString(state))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(page))
}

// githubManifestCallback verifies the manifest state, exchanges the temporary
// code for App credentials, persists them, and redirects to the frontend.
func (s *Server) githubManifestCallback(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if err := s.manifest.VerifyState(state, principal.TenantID); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid manifest state")
		return
	}
	if _, err := s.manifest.ExchangeCode(r.Context(), principal.TenantID, code); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	http.Redirect(w, r, "/git-integration?connected=1", http.StatusFound)
}

// githubAppInstall redirects admins to GitHub's installation picker for an
// already-created App while carrying a fresh signed state token.
func (s *Server) githubAppInstall(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	view, err := s.gitHubAppManager.Get(r.Context(), principal.TenantID)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	installURL, err := s.manifest.BuildInstallURL(principal.TenantID, view.AppSlug)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	http.Redirect(w, r, installURL, http.StatusFound)
}

// githubAppInstalled captures the installation_id from GitHub's setup redirect,
// verifies state, and re-persists the tenant's App configuration with it.
func (s *Server) githubAppInstalled(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	state := r.URL.Query().Get("state")
	installationID, err := strconv.ParseInt(r.URL.Query().Get("installation_id"), 10, 64)
	if err != nil || installationID == 0 {
		writeFieldError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "installation_id is invalid", "installation_id")
		return
	}
	if err := s.manifest.VerifyState(state, principal.TenantID); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid manifest state")
		return
	}

	if _, err := s.gitHubAppManager.Install(r.Context(), principal.TenantID, installationID); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	http.Redirect(w, r, "/git-integration?installed=1", http.StatusFound)
}

// testGitHubApp checks the tenant's GitHub App connection by listing the
// installation's repositories once. It always returns HTTP 200; failures are
// reported inside the payload and never echo the private key.
func (s *Server) testGitHubApp(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)

	source, err := s.gitHubAppManager.IssueSource(r.Context(), principal.TenantID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": connectionTestResult(err)})
		return
	}
	repos, err := source.ListInstallationRepositories(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": connectionTestResult(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"ok":         true,
		"repo_count": len(repos),
	}})
}

// connectionTestResult maps an error to the failure payload, mapping the
// not-configured domain error to a stable message.
func connectionTestResult(err error) map[string]any {
	message := "connection test failed"
	if errors.Is(err, git.ErrGitHubAppNotConfigured) {
		message = "github app not configured"
	}
	return map[string]any{
		"ok":         false,
		"repo_count": 0,
		"error":      message,
	}
}
