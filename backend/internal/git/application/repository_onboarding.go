package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
)

const (
	RepositorySourceGitHubApp    = "github_app"
	RepositorySourcePublicGitHub = "public_github"
)

type RepositoryCandidateView struct {
	FullName      string
	DefaultBranch string
	Visibility    string
}

type OnboardedRepositoryView struct {
	ID            string
	SourceType    string
	GitHubAppID   string
	FullName      string
	DefaultBranch string
	Visibility    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RepositoryOnboardingSummary struct {
	GitHubApp             GitHubAppView
	AppRepositories       []RepositoryCandidateView
	OnboardedRepositories []OnboardedRepositoryView
	AppRepositoriesError  string
}

type RepositoryOnboardingService struct {
	store  OnboardedRepositoryStore
	apps   GitHubAppManager
	public PublicRepositoryResolver
	newID  func() string
}

func NewRepositoryOnboardingService(store OnboardedRepositoryStore, apps GitHubAppManager, public PublicRepositoryResolver, newID func() string) (*RepositoryOnboardingService, error) {
	if store == nil {
		return nil, invalid("repository_store")
	}
	if apps == nil {
		return nil, invalid("github_app_manager")
	}
	if public == nil {
		return nil, invalid("public_repository_resolver")
	}
	if newID == nil {
		newID = randomID
	}
	return &RepositoryOnboardingService{store: store, apps: apps, public: public, newID: newID}, nil
}

func (s *RepositoryOnboardingService) AddPublicRepository(ctx context.Context, principal Principal, input string) (OnboardedRepositoryView, error) {
	var view OnboardedRepositoryView
	if principal.TenantID == "" {
		return view, invalid("tenant_id")
	}
	if err := requireRepositoryOnboardingAdmin(principal); err != nil {
		return view, err
	}
	fullName, err := normalizeGitHubRepository(input)
	if err != nil {
		return view, err
	}
	repo, err := s.public.ResolvePublicRepository(ctx, fullName)
	if err != nil {
		return view, err
	}
	if err := s.rejectDuplicateRepository(ctx, principal.TenantID, repo.FullName); err != nil {
		return view, err
	}
	record := &OnboardedRepositoryRecord{
		ID:            s.newID(),
		TenantID:      principal.TenantID,
		SourceType:    RepositorySourcePublicGitHub,
		FullName:      repo.FullName,
		DefaultBranch: repo.DefaultBranch,
		Visibility:    repo.Visibility,
	}
	if err := s.store.UpsertOnboardedRepository(ctx, record); err != nil {
		return view, err
	}
	return toOnboardedRepositoryView(*record), nil
}

func (s *RepositoryOnboardingService) ListGitHubAppRepositories(ctx context.Context, principal Principal, appID string) ([]RepositoryCandidateView, error) {
	if principal.TenantID == "" {
		return nil, invalid("tenant_id")
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, invalid("github_app_id")
	}
	source, err := s.apps.IssueSourceForApp(ctx, principal.TenantID, appID)
	if err != nil {
		if errors.Is(err, git.ErrGitHubAppNotConfigured) {
			return nil, notFound()
		}
		return nil, err
	}
	repos, err := source.ListInstallationRepositories(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]RepositoryCandidateView, 0, len(repos))
	for _, repo := range repos {
		items = append(items, RepositoryCandidateView{
			FullName:      repo.FullName,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
		})
	}
	return items, nil
}

func (s *RepositoryOnboardingService) AddGitHubAppRepository(ctx context.Context, principal Principal, appID, fullName string) (OnboardedRepositoryView, error) {
	var view OnboardedRepositoryView
	if principal.TenantID == "" {
		return view, invalid("tenant_id")
	}
	if err := requireRepositoryOnboardingAdmin(principal); err != nil {
		return view, err
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return view, invalid("github_app_id")
	}
	normalized, err := normalizeGitHubRepository(fullName)
	if err != nil {
		return view, err
	}
	source, err := s.apps.IssueSourceForApp(ctx, principal.TenantID, appID)
	if err != nil {
		if errors.Is(err, git.ErrGitHubAppNotConfigured) {
			return view, notFound()
		}
		return view, err
	}
	repos, err := source.ListInstallationRepositories(ctx)
	if err != nil {
		return view, err
	}
	for _, repo := range repos {
		if repo.FullName != normalized {
			continue
		}
		if err := s.rejectDuplicateRepository(ctx, principal.TenantID, repo.FullName); err != nil {
			return view, err
		}
		record := &OnboardedRepositoryRecord{
			ID:            s.newID(),
			TenantID:      principal.TenantID,
			SourceType:    RepositorySourceGitHubApp,
			GitHubAppID:   appID,
			FullName:      repo.FullName,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
		}
		if err := s.store.UpsertOnboardedRepository(ctx, record); err != nil {
			return view, err
		}
		return toOnboardedRepositoryView(*record), nil
	}
	return view, notFound()
}

func (s *RepositoryOnboardingService) rejectDuplicateRepository(ctx context.Context, tenantID, fullName string) error {
	records, err := s.store.ListOnboardedRepositories(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.FullName == fullName {
			return git.ErrRepositoryBindingConflict
		}
	}
	return nil
}

func (s *RepositoryOnboardingService) Summary(ctx context.Context, principal Principal) (RepositoryOnboardingSummary, error) {
	var summary RepositoryOnboardingSummary
	if principal.TenantID == "" {
		return summary, invalid("tenant_id")
	}
	records, err := s.store.ListOnboardedRepositories(ctx, principal.TenantID)
	if err != nil {
		return summary, err
	}
	summary.OnboardedRepositories = make([]OnboardedRepositoryView, 0, len(records))
	for _, record := range records {
		summary.OnboardedRepositories = append(summary.OnboardedRepositories, toOnboardedRepositoryView(record))
	}

	appView, err := s.apps.Get(ctx, principal.TenantID)
	if err != nil {
		if errors.Is(err, git.ErrGitHubAppNotConfigured) {
			summary.GitHubApp = GitHubAppView{Configured: false}
			return summary, nil
		}
		return summary, err
	}
	summary.GitHubApp = appView
	if !appView.Configured {
		return summary, nil
	}

	source, err := s.apps.IssueSource(ctx, principal.TenantID)
	if err != nil {
		summary.AppRepositoriesError = err.Error()
		return summary, nil
	}
	repos, err := source.ListInstallationRepositories(ctx)
	if err != nil {
		summary.AppRepositoriesError = err.Error()
		return summary, nil
	}
	summary.AppRepositories = make([]RepositoryCandidateView, 0, len(repos))
	for _, repo := range repos {
		summary.AppRepositories = append(summary.AppRepositories, RepositoryCandidateView{
			FullName:      repo.FullName,
			DefaultBranch: repo.DefaultBranch,
			Visibility:    repo.Visibility,
		})
	}
	return summary, nil
}

func (s *RepositoryOnboardingService) Remove(ctx context.Context, principal Principal, id string) error {
	if principal.TenantID == "" {
		return invalid("tenant_id")
	}
	if err := requireRepositoryOnboardingAdmin(principal); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return invalid("repository_id")
	}
	return s.store.DeleteOnboardedRepository(ctx, principal.TenantID, id)
}

func normalizeGitHubRepository(input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", invalid("repo")
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", invalid("repo")
		}
		if parsed.Scheme != "https" || parsed.Host != "github.com" {
			return "", invalid("repo")
		}
		value = strings.Trim(parsed.Path, "/")
	}
	if strings.HasSuffix(value, ".git") {
		value = strings.TrimSuffix(value, ".git")
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.HasPrefix(input, "/") {
		return "", invalid("repo")
	}
	return parts[0] + "/" + parts[1], nil
}

func notFound() error {
	return &domain.Error{Code: "not_found", Message: "resource not found"}
}

func requireRepositoryOnboardingAdmin(principal Principal) error {
	if !principal.IsAdmin {
		return &domain.Error{Code: "forbidden", Message: "admin privileges are required"}
	}
	return nil
}

func toOnboardedRepositoryView(record OnboardedRepositoryRecord) OnboardedRepositoryView {
	return OnboardedRepositoryView{
		ID:            record.ID,
		SourceType:    record.SourceType,
		GitHubAppID:   record.GitHubAppID,
		FullName:      record.FullName,
		DefaultBranch: record.DefaultBranch,
		Visibility:    record.Visibility,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}
