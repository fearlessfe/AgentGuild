package acceptance

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

// acceptanceGitAppService wraps a fixed fake git.Driver as a repository resolver.
// It is used by acceptance tests where the git backend is mocked per-tenant.
type acceptanceGitAppService struct {
	driver git.Driver
}

func (s *acceptanceGitAppService) Driver(ctx context.Context, tenantID, repo string) (git.ResolvedDriver, error) {
	return git.ResolvedDriver{Driver: s.driver, FullName: repo}, nil
}

func (s *acceptanceGitAppService) IssueSource(context.Context, string, string, string) (git.ResolvedIssueSource, error) {
	return git.ResolvedIssueSource{}, nil
}

var _ gitapp.RepositoryGitResolver = (*acceptanceGitAppService)(nil)
