package acceptance

import (
	"context"
	"fmt"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

// acceptanceGitDriver 是验收测试用的 fake git 平台驱动。
// 它返回固定数据，不访问真实 GitHub API，但仓库名使用真实 GitHub 开源项目。
type acceptanceGitDriver struct {
	repo        string
	baseCommit  string
	headCommit  string
	branch      string
	changedFile string
}

func newAcceptanceGitDriver(repo, baseCommit, headCommit, branch, changedFile string) *acceptanceGitDriver {
	return &acceptanceGitDriver{
		repo:        repo,
		baseCommit:  baseCommit,
		headCommit:  headCommit,
		branch:      branch,
		changedFile: changedFile,
	}
}

func (d *acceptanceGitDriver) CreateCredential(_ context.Context, repo, branch, baseCommit string) (git.Credential, error) {
	return git.Credential{
		Token:      "acceptance-git-token",
		RepoURL:    fmt.Sprintf("https://github.com/%s.git", repo),
		Branch:     branch,
		BaseCommit: baseCommit,
		ExpiresAt:  time.Now().Add(15 * time.Minute),
	}, nil
}

func (d *acceptanceGitDriver) GetCommit(_ context.Context, repo, sha string) (git.Commit, error) {
	if repo != d.repo {
		return git.Commit{}, git.ErrRepoNotFound
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	switch sha {
	case d.baseCommit, d.headCommit, d.branch:
		return git.Commit{
			SHA:         sha,
			Message:     "acceptance commit",
			Author:      "Acceptance Bot",
			Email:       "acceptance@example.test",
			CommittedAt: now,
			URL:         fmt.Sprintf("https://github.com/%s/commit/%s", repo, sha),
		}, nil
	}
	return git.Commit{}, git.ErrRepoNotFound
}

func (d *acceptanceGitDriver) CompareCommits(_ context.Context, repo, base, head string) ([]git.ChangedFile, error) {
	if repo != d.repo {
		return nil, git.ErrRepoNotFound
	}
	if base != d.baseCommit || head != d.headCommit {
		return nil, git.ErrRepoNotFound
	}
	return []git.ChangedFile{
		{
			Filename:  d.changedFile,
			Status:    "modified",
			Additions: 10,
			Deletions: 2,
			Patch:     "@@ -1,2 +1,3 @@\n old\n+new",
		},
	}, nil
}

func (d *acceptanceGitDriver) IsAncestor(_ context.Context, repo, base, head string) (bool, error) {
	if repo != d.repo {
		return false, git.ErrRepoNotFound
	}
	if base == d.baseCommit && head == d.headCommit {
		return true, nil
	}
	if base == d.headCommit && head == d.headCommit {
		return true, nil
	}
	if base == d.baseCommit && head == d.branch {
		return true, nil
	}
	return false, nil
}

var _ git.Driver = (*acceptanceGitDriver)(nil)
