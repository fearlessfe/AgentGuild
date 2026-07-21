package rest

import (
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/go-chi/chi/v5"
)

type repositoryOnboardingSummaryView struct {
	GitHubApp             gitapp.GitHubAppView `json:"github_app"`
	AppRepositories       repositoryItemsView  `json:"app_repositories"`
	OnboardedRepositories repositoryItemsView  `json:"onboarded_repositories"`
	AppRepositoriesError  string               `json:"app_repositories_error,omitempty"`
}

type repositoryItemsView struct {
	Items []repositoryItemView `json:"items"`
}

type repositoryItemView struct {
	ID            string     `json:"id,omitempty"`
	SourceType    string     `json:"source_type,omitempty"`
	GitHubAppID   string     `json:"github_app_id,omitempty"`
	FullName      string     `json:"full_name"`
	DefaultBranch string     `json:"default_branch"`
	Visibility    string     `json:"visibility"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type addRepositoryBody struct {
	GitHubAppID string `json:"github_app_id"`
	Repo        string `json:"repo"`
}

func (s *Server) listGitHubAppRepositories(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	items, err := s.repositoryOnboarding.ListGitHubAppRepositories(r.Context(), repositoryOnboardingPrincipal(principal), chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, repositoryItemsView{Items: toRepositoryCandidateItemViews(items)})
}

func (s *Server) getRepositoryOnboarding(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	summary, err := s.repositoryOnboarding.Summary(r.Context(), repositoryOnboardingPrincipal(principal))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, toRepositoryOnboardingSummaryView(summary))
}

func (s *Server) addGitHubAppRepository(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	var body addRepositoryBody
	if !decodeBody(w, r, &body) {
		return
	}
	view, err := s.repositoryOnboarding.AddGitHubAppRepository(r.Context(), repositoryOnboardingPrincipal(principal), body.GitHubAppID, body.Repo)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusCreated, toOnboardedRepositoryItemView(view))
}

func (s *Server) addPublicRepository(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	var body addRepositoryBody
	if !decodeBody(w, r, &body) {
		return
	}
	view, err := s.repositoryOnboarding.AddPublicRepository(r.Context(), repositoryOnboardingPrincipal(principal), body.Repo)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusCreated, toOnboardedRepositoryItemView(view))
}

func (s *Server) deleteOnboardedRepository(w http.ResponseWriter, r *http.Request) {
	principal := mustPrincipal(r)
	if !requireAdminSyncSession(w, principal) {
		return
	}
	if err := s.repositoryOnboarding.Remove(r.Context(), repositoryOnboardingPrincipal(principal), chi.URLParam(r, "id")); err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]bool{"deleted": true})
}

func repositoryOnboardingPrincipal(principal auth.Principal) gitapp.Principal {
	return gitapp.Principal{
		SubjectID:      principal.SubjectID,
		IdentityScope:  principal.IdentityScope,
		TenantID:       principal.TenantID,
		OwnerID:        principal.OwnerID,
		OwnerEmail:     principal.OwnerEmail,
		IsAdmin:        principal.IsAdmin,
		AgentID:        principal.AgentID,
		AgentVersionID: principal.AgentVersionID,
		Scopes:         append([]string(nil), principal.Scopes...),
		RepoScope:      append([]string(nil), principal.RepoScope...),
	}
}

func toRepositoryOnboardingSummaryView(summary gitapp.RepositoryOnboardingSummary) repositoryOnboardingSummaryView {
	return repositoryOnboardingSummaryView{
		GitHubApp:             summary.GitHubApp,
		AppRepositories:       repositoryItemsView{Items: toRepositoryCandidateItemViews(summary.AppRepositories)},
		OnboardedRepositories: repositoryItemsView{Items: toOnboardedRepositoryItemViews(summary.OnboardedRepositories)},
		AppRepositoriesError:  summary.AppRepositoriesError,
	}
}

func toRepositoryCandidateItemViews(items []gitapp.RepositoryCandidateView) []repositoryItemView {
	views := make([]repositoryItemView, 0, len(items))
	for _, item := range items {
		views = append(views, repositoryItemView{
			FullName:      item.FullName,
			DefaultBranch: item.DefaultBranch,
			Visibility:    item.Visibility,
		})
	}
	return views
}

func toOnboardedRepositoryItemViews(items []gitapp.OnboardedRepositoryView) []repositoryItemView {
	views := make([]repositoryItemView, 0, len(items))
	for _, item := range items {
		views = append(views, toOnboardedRepositoryItemView(item))
	}
	return views
}

func toOnboardedRepositoryItemView(item gitapp.OnboardedRepositoryView) repositoryItemView {
	view := repositoryItemView{
		ID:            item.ID,
		SourceType:    item.SourceType,
		GitHubAppID:   item.GitHubAppID,
		FullName:      item.FullName,
		DefaultBranch: item.DefaultBranch,
		Visibility:    item.Visibility,
	}
	if !item.CreatedAt.IsZero() {
		view.CreatedAt = &item.CreatedAt
	}
	if !item.UpdatedAt.IsZero() {
		view.UpdatedAt = &item.UpdatedAt
	}
	return view
}
