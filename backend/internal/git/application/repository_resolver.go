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
	Driver(ctx context.Context, tenantID, fullName string) (git.ResolvedDriver, error)
	IssueSource(ctx context.Context, tenantID, fullName, sourceAuth string) (git.ResolvedIssueSource, error)
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

func (r *repositoryGitResolver) Driver(ctx context.Context, tenantID, fullName string) (git.ResolvedDriver, error) {
	record, canonical, err := r.repository(ctx, tenantID, fullName)
	if err != nil {
		return git.ResolvedDriver{}, err
	}
	if record.SourceType != RepositorySourceGitHubApp || record.GitHubAppID == "" {
		return git.ResolvedDriver{}, repositoryAccessConflict()
	}
	driver, err := r.apps.DriverForApp(ctx, tenantID, record.GitHubAppID)
	if err != nil {
		return git.ResolvedDriver{}, err
	}
	return git.ResolvedDriver{Driver: driver, FullName: canonical}, nil
}

func (r *repositoryGitResolver) ResolveBaseCommit(ctx context.Context, tenantID, fullName string) (string, error) {
	record, canonical, err := r.repository(ctx, tenantID, fullName)
	if err != nil {
		return "", err
	}
	if record.SourceType != RepositorySourceGitHubApp || record.GitHubAppID == "" || record.DefaultBranch == "" {
		return "", repositoryAccessConflict()
	}
	driver, err := r.apps.DriverForApp(ctx, tenantID, record.GitHubAppID)
	if err != nil {
		return "", err
	}
	commit, err := driver.GetCommit(ctx, canonical, record.DefaultBranch)
	if err != nil {
		return "", err
	}
	if commit.SHA == "" {
		return "", repositoryAccessConflict()
	}
	return commit.SHA, nil
}

var _ RepositoryBaseResolver = (*repositoryGitResolver)(nil)

func (r *repositoryGitResolver) IssueSource(ctx context.Context, tenantID, fullName, sourceAuth string) (git.ResolvedIssueSource, error) {
	record, canonical, err := r.repository(ctx, tenantID, fullName)
	if err != nil {
		return git.ResolvedIssueSource{}, err
	}
	var source git.IssueSource
	switch sourceAuth {
	case "", repositoryIssueSourceAuthApp:
		if record.SourceType != RepositorySourceGitHubApp || record.GitHubAppID == "" {
			return git.ResolvedIssueSource{}, repositoryAccessConflict()
		}
		source, err = r.apps.IssueSourceForApp(ctx, tenantID, record.GitHubAppID)
	case repositoryIssueSourceAuthPublic:
		if record.SourceType != RepositorySourcePublicGitHub {
			return git.ResolvedIssueSource{}, repositoryAccessConflict()
		}
		source = r.public
	default:
		return git.ResolvedIssueSource{}, invalid("source_auth")
	}
	if err != nil {
		return git.ResolvedIssueSource{}, err
	}
	return git.ResolvedIssueSource{Source: source, FullName: canonical}, nil
}

func (r *repositoryGitResolver) repository(ctx context.Context, tenantID, fullName string) (*OnboardedRepositoryRecord, string, error) {
	if tenantID == "" {
		return nil, "", invalid("tenant_id")
	}
	normalized, err := normalizeGitHubRepository(fullName)
	if err != nil {
		return nil, "", err
	}
	record, err := r.store.GetOnboardedRepositoryByFullName(ctx, tenantID, normalized)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) || errors.Is(err, git.ErrRepoNotFound) {
			return nil, "", repositoryNotFound()
		}
		return nil, "", err
	}
	if record == nil {
		return nil, "", repositoryNotFound()
	}
	return record, normalized, nil
}

func repositoryNotFound() error {
	return &domain.Error{Code: "not_found", Message: "repository is not onboarded", Field: "repo"}
}

func repositoryAccessConflict() error {
	return &domain.Error{Code: "state_conflict", Message: "repository binding does not allow this access mode", Field: "repo"}
}
