package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/github"
)

// GitHubAppRecord is the persistent configuration for a tenant's GitHub App.
type GitHubAppRecord struct {
	TenantID       string
	Provider       string
	AppID          int64
	InstallationID int64
	PrivateKey     string
	BaseURL        string
	WebhookSecret  string
	ClientID       string
	ClientSecret   string
	AppSlug        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GitHubAppView is the public shape of a GitHub App configuration. It never
// includes the private key plaintext.
type GitHubAppView struct {
	TenantID       string    `json:"tenant_id"`
	Provider       string    `json:"provider"`
	AppID          int64     `json:"app_id"`
	InstallationID int64     `json:"installation_id"`
	BaseURL        string    `json:"base_url"`
	AppSlug        string    `json:"app_slug,omitempty"`
	Configured     bool      `json:"configured"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UpsertGitHubApp creates or replaces a tenant's GitHub App configuration.
type UpsertGitHubApp struct {
	TenantID       string
	Provider       string
	AppID          int64
	InstallationID int64
	PrivateKey     string
	BaseURL        string
	WebhookSecret  string
	ClientID       string
	ClientSecret   string
	AppSlug        string
}

// gitHubAppService is the concrete implementation of GitHubAppService.
type gitHubAppService struct {
	repo GitHubAppRepository
}

// NewGitHubAppManager creates a GitHubAppManager.
func NewGitHubAppManager(repo GitHubAppRepository) (GitHubAppManager, error) {
	if repo == nil {
		return nil, invalid("github_app_repository")
	}
	return &gitHubAppService{repo: repo}, nil
}

// NewGitHubAppService creates a GitHubAppService from a repository.
func NewGitHubAppService(repo GitHubAppRepository) (GitHubAppService, error) {
	return NewGitHubAppManager(repo)
}

// Upsert persists the GitHub App configuration for a tenant.
func (s *gitHubAppService) Upsert(ctx context.Context, cmd UpsertGitHubApp) error {
	if cmd.TenantID == "" {
		return invalid("tenant_id")
	}
	if cmd.AppID == 0 {
		return invalid("app_id")
	}
	if cmd.PrivateKey == "" {
		return invalid("private_key")
	}
	provider := cmd.Provider
	if provider == "" {
		provider = "github"
	}
	baseURL := cmd.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return s.repo.Upsert(ctx, &GitHubAppRecord{
		TenantID:       cmd.TenantID,
		Provider:       provider,
		AppID:          cmd.AppID,
		InstallationID: cmd.InstallationID,
		PrivateKey:     cmd.PrivateKey,
		BaseURL:        baseURL,
		WebhookSecret:  cmd.WebhookSecret,
		ClientID:       cmd.ClientID,
		ClientSecret:   cmd.ClientSecret,
		AppSlug:        cmd.AppSlug,
	})
}

// Get returns the public view of a tenant's GitHub App configuration.
func (s *gitHubAppService) Get(ctx context.Context, tenantID string) (GitHubAppView, error) {
	var view GitHubAppView
	if tenantID == "" {
		return view, invalid("tenant_id")
	}
	record, err := s.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		return view, err
	}
	return toGitHubAppView(record), nil
}

// Install records the GitHub App installation selected on GitHub while
// preserving the App credentials returned by the manifest conversion.
func (s *gitHubAppService) Install(ctx context.Context, tenantID string, installationID int64) (GitHubAppView, error) {
	if tenantID == "" {
		return GitHubAppView{}, invalid("tenant_id")
	}
	if installationID == 0 {
		return GitHubAppView{}, invalid("installation_id")
	}
	record, err := s.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		return GitHubAppView{}, err
	}
	copy := *record
	copy.InstallationID = installationID
	if err := s.repo.Upsert(ctx, &copy); err != nil {
		return GitHubAppView{}, err
	}
	return s.Get(ctx, tenantID)
}

// Delete removes a tenant's GitHub App configuration.
func (s *gitHubAppService) Delete(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return invalid("tenant_id")
	}
	return s.repo.Delete(ctx, tenantID)
}

// Driver returns a git.Driver for the tenant, or an error if not configured.
func (s *gitHubAppService) Driver(ctx context.Context, tenantID string) (git.Driver, error) {
	if tenantID == "" {
		return nil, invalid("tenant_id")
	}
	record, err := s.repo.GetByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return github.NewDriver(github.Config{
		AppID:          record.AppID,
		PrivateKey:     record.PrivateKey,
		InstallationID: record.InstallationID,
		BaseURL:        record.BaseURL,
	})
}

// IssueSource returns a git.IssueSource for the tenant's configured installation.
func (s *gitHubAppService) IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error) {
	driver, err := s.Driver(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	source, ok := driver.(git.IssueSource)
	if !ok {
		return nil, invalid("issue_source")
	}
	return source, nil
}

func toGitHubAppView(record *GitHubAppRecord) GitHubAppView {
	if record == nil {
		return GitHubAppView{Configured: false}
	}
	return GitHubAppView{
		TenantID:       record.TenantID,
		Provider:       record.Provider,
		AppID:          record.AppID,
		InstallationID: record.InstallationID,
		BaseURL:        record.BaseURL,
		AppSlug:        record.AppSlug,
		Configured:     true,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}
