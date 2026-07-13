package application

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/github"
)

// GitHubAppRecord is the persistent configuration for a tenant's GitHub App.
type GitHubAppRecord struct {
	ID                       string
	TenantID                 string
	Provider                 string
	AppID                    int64
	InstallationID           int64
	PrivateKey               string
	BaseURL                  string
	WebhookSecret            string
	ClientID                 string
	ClientSecret             string
	AppSlug                  string
	InstallationAccountLogin string
	IsDefault                bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// GitHubAppView is the public shape of a GitHub App configuration. It never
// includes the private key plaintext.
type GitHubAppView struct {
	ID                       string    `json:"id"`
	TenantID                 string    `json:"tenant_id"`
	Provider                 string    `json:"provider"`
	AppID                    int64     `json:"app_id"`
	InstallationID           int64     `json:"installation_id"`
	BaseURL                  string    `json:"base_url"`
	AppSlug                  string    `json:"app_slug,omitempty"`
	InstallationAccountLogin string    `json:"installation_account_login,omitempty"`
	IsDefault                bool      `json:"is_default"`
	Configured               bool      `json:"configured"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// UpsertGitHubApp creates or replaces a tenant's GitHub App configuration.
type UpsertGitHubApp struct {
	ID                       string
	TenantID                 string
	Provider                 string
	AppID                    int64
	InstallationID           int64
	PrivateKey               string
	BaseURL                  string
	WebhookSecret            string
	ClientID                 string
	ClientSecret             string
	AppSlug                  string
	InstallationAccountLogin string
}

// gitHubAppService is the concrete implementation of GitHubAppService.
type gitHubAppService struct {
	repo  GitHubAppRepository
	newID func() string
}

// GitHubAppManagerOptions configures GitHub App management.
type GitHubAppManagerOptions struct {
	NewID func() string
}

// NewGitHubAppManager creates a GitHubAppManager.
func NewGitHubAppManager(repo GitHubAppRepository) (GitHubAppManager, error) {
	return NewGitHubAppManagerWithOptions(repo, GitHubAppManagerOptions{})
}

// NewGitHubAppManagerWithOptions creates a GitHubAppManager with injectable dependencies.
func NewGitHubAppManagerWithOptions(repo GitHubAppRepository, options GitHubAppManagerOptions) (GitHubAppManager, error) {
	if repo == nil {
		return nil, invalid("github_app_repository")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &gitHubAppService{repo: repo, newID: options.NewID}, nil
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
	id := cmd.ID
	if id == "" {
		id = s.newID()
	}
	if id == "" {
		return invalid("id")
	}
	isDefault := false
	existing, err := s.repo.GetByID(ctx, cmd.TenantID, id)
	switch {
	case err == nil:
		isDefault = existing.IsDefault
	case errors.Is(err, git.ErrGitHubAppNotConfigured):
		records, listErr := s.repo.ListByTenant(ctx, cmd.TenantID)
		if listErr != nil {
			return listErr
		}
		isDefault = len(records) == 0
	default:
		return err
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
		ID:                       id,
		TenantID:                 cmd.TenantID,
		Provider:                 provider,
		AppID:                    cmd.AppID,
		InstallationID:           cmd.InstallationID,
		PrivateKey:               cmd.PrivateKey,
		BaseURL:                  baseURL,
		WebhookSecret:            cmd.WebhookSecret,
		ClientID:                 cmd.ClientID,
		ClientSecret:             cmd.ClientSecret,
		AppSlug:                  cmd.AppSlug,
		InstallationAccountLogin: cmd.InstallationAccountLogin,
		IsDefault:                isDefault,
	})
}

// List returns all public GitHub App configurations for a tenant.
func (s *gitHubAppService) List(ctx context.Context, tenantID string) ([]GitHubAppView, error) {
	if tenantID == "" {
		return nil, invalid("tenant_id")
	}
	records, err := s.repo.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	views := make([]GitHubAppView, 0, len(records))
	for i := range records {
		views = append(views, toGitHubAppView(&records[i]))
	}
	return views, nil
}

// Get returns the public view of a tenant's GitHub App configuration.
func (s *gitHubAppService) Get(ctx context.Context, tenantID string) (GitHubAppView, error) {
	var view GitHubAppView
	if tenantID == "" {
		return view, invalid("tenant_id")
	}
	record, err := s.repo.GetDefault(ctx, tenantID)
	if err != nil {
		return view, err
	}
	return toGitHubAppView(record), nil
}

// GetByID returns one tenant-scoped GitHub App public view.
func (s *gitHubAppService) GetByID(ctx context.Context, tenantID, appID string) (GitHubAppView, error) {
	if tenantID == "" {
		return GitHubAppView{}, invalid("tenant_id")
	}
	if appID == "" {
		return GitHubAppView{}, invalid("github_app_id")
	}
	record, err := s.repo.GetByID(ctx, tenantID, appID)
	if err != nil {
		return GitHubAppView{}, err
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
	record, err := s.repo.GetDefault(ctx, tenantID)
	if err != nil {
		return GitHubAppView{}, err
	}
	return s.InstallByID(ctx, tenantID, record.ID, installationID, record.InstallationAccountLogin)
}

// InstallByID records installation metadata for one tenant-scoped GitHub App.
func (s *gitHubAppService) InstallByID(ctx context.Context, tenantID, appID string, installationID int64, accountLogin string) (GitHubAppView, error) {
	if tenantID == "" {
		return GitHubAppView{}, invalid("tenant_id")
	}
	if appID == "" {
		return GitHubAppView{}, invalid("github_app_id")
	}
	if installationID == 0 {
		return GitHubAppView{}, invalid("installation_id")
	}
	record, err := s.repo.GetByID(ctx, tenantID, appID)
	if err != nil {
		return GitHubAppView{}, err
	}
	copy := *record
	copy.InstallationID = installationID
	copy.InstallationAccountLogin = accountLogin
	if err := s.repo.Upsert(ctx, &copy); err != nil {
		return GitHubAppView{}, err
	}
	return s.GetByID(ctx, tenantID, appID)
}

// Delete removes a tenant's GitHub App configuration.
func (s *gitHubAppService) Delete(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return invalid("tenant_id")
	}
	record, err := s.repo.GetDefault(ctx, tenantID)
	if err != nil {
		return err
	}
	return s.DeleteByID(ctx, tenantID, record.ID)
}

// DeleteByID atomically removes an unbound App and promotes a replacement default.
func (s *gitHubAppService) DeleteByID(ctx context.Context, tenantID, appID string) error {
	if tenantID == "" {
		return invalid("tenant_id")
	}
	if appID == "" {
		return invalid("github_app_id")
	}
	return s.repo.DeleteAndPromoteDefault(ctx, tenantID, appID)
}

// Driver returns a git.Driver for the tenant, or an error if not configured.
func (s *gitHubAppService) Driver(ctx context.Context, tenantID string) (git.Driver, error) {
	if tenantID == "" {
		return nil, invalid("tenant_id")
	}
	record, err := s.repo.GetDefault(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return driverFromGitHubApp(record)
}

// DriverForApp returns a git.Driver for one tenant-scoped GitHub App.
func (s *gitHubAppService) DriverForApp(ctx context.Context, tenantID, appID string) (git.Driver, error) {
	if tenantID == "" {
		return nil, invalid("tenant_id")
	}
	if appID == "" {
		return nil, invalid("github_app_id")
	}
	record, err := s.repo.GetByID(ctx, tenantID, appID)
	if err != nil {
		return nil, err
	}
	return driverFromGitHubApp(record)
}

func driverFromGitHubApp(record *GitHubAppRecord) (git.Driver, error) {
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

// IssueSourceForApp returns an IssueSource for one tenant-scoped GitHub App.
func (s *gitHubAppService) IssueSourceForApp(ctx context.Context, tenantID, appID string) (git.IssueSource, error) {
	driver, err := s.DriverForApp(ctx, tenantID, appID)
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
		ID:                       record.ID,
		TenantID:                 record.TenantID,
		Provider:                 record.Provider,
		AppID:                    record.AppID,
		InstallationID:           record.InstallationID,
		BaseURL:                  record.BaseURL,
		AppSlug:                  record.AppSlug,
		InstallationAccountLogin: record.InstallationAccountLogin,
		IsDefault:                record.IsDefault,
		Configured:               true,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
	}
}
