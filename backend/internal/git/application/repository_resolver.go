package application

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
)

const (
	repositoryIssueSourceAuthApp    = "app"
	repositoryIssueSourceAuthPublic = "public"
)

// RepositoryGitResolver resolves runtime Git access from the repository's
// immutable tenant-scoped onboarding binding.
type RepositoryGitResolver interface {
	Driver(ctx context.Context, tenantID, fullName string) (git.Driver, error)
	IssueSource(ctx context.Context, tenantID, fullName, sourceAuth string) (git.IssueSource, error)
}

type repositoryGitAppProvider interface {
	DriverForApp(context.Context, string, string) (git.Driver, error)
	IssueSourceForApp(context.Context, string, string) (git.IssueSource, error)
}

type repositoryGitResolver struct {
	store  OnboardedRepositoryStore
	apps   repositoryGitAppProvider
	public git.IssueSource
}

// NewRepositoryGitResolver creates the single repository-scoped runtime Git
// resolver used by credentials, commit verification, and issue sync.
func NewRepositoryGitResolver(store OnboardedRepositoryStore, apps repositoryGitAppProvider, public git.IssueSource) (RepositoryGitResolver, error) {
	if store == nil {
		return nil, invalid("repository_store")
	}
	if apps == nil {
		return nil, invalid("github_app_manager")
	}
	if public == nil {
		return nil, invalid("public_issue_source")
	}
	return &repositoryGitResolver{store: store, apps: apps, public: public}, nil
}

func (r *repositoryGitResolver) Driver(ctx context.Context, tenantID, fullName string) (git.Driver, error) {
	record, err := r.repository(ctx, tenantID, fullName)
	if err != nil {
		return nil, err
	}
	if record.SourceType != RepositorySourceGitHubApp || record.GitHubAppID == "" {
		return nil, repositoryAccessConflict()
	}
	return r.apps.DriverForApp(ctx, tenantID, record.GitHubAppID)
}

func (r *repositoryGitResolver) IssueSource(ctx context.Context, tenantID, fullName, sourceAuth string) (git.IssueSource, error) {
	record, err := r.repository(ctx, tenantID, fullName)
	if err != nil {
		return nil, err
	}
	switch sourceAuth {
	case "", repositoryIssueSourceAuthApp:
		if record.SourceType != RepositorySourceGitHubApp || record.GitHubAppID == "" {
			return nil, repositoryAccessConflict()
		}
		return r.apps.IssueSourceForApp(ctx, tenantID, record.GitHubAppID)
	case repositoryIssueSourceAuthPublic:
		if record.SourceType != RepositorySourcePublicGitHub {
			return nil, repositoryAccessConflict()
		}
		return r.public, nil
	default:
		return nil, invalid("source_auth")
	}
}

func (r *repositoryGitResolver) repository(ctx context.Context, tenantID, fullName string) (*OnboardedRepositoryRecord, error) {
	if tenantID == "" {
		return nil, invalid("tenant_id")
	}
	normalized, err := normalizeGitHubRepository(fullName)
	if err != nil {
		return nil, err
	}
	record, err := r.store.GetOnboardedRepositoryByFullName(ctx, tenantID, normalized)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, git.ErrRepoNotFound) {
			return nil, repositoryNotFound()
		}
		return nil, err
	}
	if record == nil {
		return nil, repositoryNotFound()
	}
	return record, nil
}

func repositoryNotFound() error {
	return &domain.Error{Code: "not_found", Message: "repository is not onboarded", Field: "repo"}
}

func repositoryAccessConflict() error {
	return &domain.Error{Code: "state_conflict", Message: "repository binding does not allow this access mode", Field: "repo"}
}
