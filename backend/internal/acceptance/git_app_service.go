package acceptance

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

// acceptanceGitAppService wraps a fixed fake git.Driver as a GitHubAppService.
// It is used by acceptance tests where the git backend is mocked per-tenant.
type acceptanceGitAppService struct {
	driver git.Driver
}

func (s *acceptanceGitAppService) Driver(ctx context.Context, tenantID string) (git.Driver, error) {
	return s.driver, nil
}

var _ gitapp.GitHubAppService = (*acceptanceGitAppService)(nil)
