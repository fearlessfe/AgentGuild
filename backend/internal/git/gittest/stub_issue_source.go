// Package gittest provides test doubles for git integrations.
package gittest

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

// StubIssueSource is an in-memory git.IssueSource for tests. It never calls GitHub.
type StubIssueSource struct {
	Repos  []git.Repository
	Issues map[string][]git.Issue
	Err    error
	Calls  int
}

func (s *StubIssueSource) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Repos, nil
}

func (s *StubIssueSource) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	s.Calls++
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Issues[repo], nil
}

var _ git.IssueSource = (*StubIssueSource)(nil)
