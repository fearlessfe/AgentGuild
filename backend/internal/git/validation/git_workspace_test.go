package validation_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"agentguild.dev/agentguild/backend/internal/git/validation"
	"github.com/stretchr/testify/require"
)

func TestGitWorkspaceFactoryRejectsLocalFileCloneURL(t *testing.T) {
	sha := "7c3f4e9a8b2d1c0f5e6a7b8c9d0e1f2a3b4c5d6e"
	resolver := &workspaceResolver{driver: &workspaceDriver{repoURL: "file:///tmp/repo", canonicalSHA: sha}}
	factory, err := validation.NewGitWorkspaceFactory(resolver)
	require.NoError(t, err)

	_, _, err = factory.Prepare(context.Background(), &gitdomain.ValidationJob{
		TenantID: "tenant-1", Repo: "owner/repo", Branch: "main", CommitSHA: sha,
	})
	require.ErrorContains(t, err, "invalid repository URL")
}

func TestGitWorkspaceFactoryRejectsDriverWithoutCanonicalCommit(t *testing.T) {
	factory, err := validation.NewGitWorkspaceFactory(&workspaceResolver{driver: &workspaceDriver{canonicalSHA: "main"}})
	require.NoError(t, err)
	_, _, err = factory.Prepare(context.Background(), &gitdomain.ValidationJob{
		TenantID: "tenant-1", Repo: "owner/repo", Branch: "main", CommitSHA: "main",
	})
	require.ErrorContains(t, err, "full immutable object id")
}

type workspaceResolver struct{ driver git.Driver }

func (r *workspaceResolver) Driver(context.Context, string, string) (git.ResolvedDriver, error) {
	return git.ResolvedDriver{Driver: r.driver, FullName: "owner/repo"}, nil
}

type workspaceDriver struct {
	repoURL      string
	canonicalSHA string
}

func (d *workspaceDriver) CreateCredential(context.Context, string, string, string) (git.Credential, error) {
	return git.Credential{Token: "test-token", RepoURL: d.repoURL, ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (d *workspaceDriver) GetCommit(context.Context, string, string) (git.Commit, error) {
	return git.Commit{SHA: d.canonicalSHA}, nil
}
func (*workspaceDriver) CompareCommits(context.Context, string, string, string) ([]git.ChangedFile, error) {
	return nil, nil
}
func (*workspaceDriver) IsAncestor(context.Context, string, string, string) (bool, error) {
	return true, nil
}
