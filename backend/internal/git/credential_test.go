package git_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"github.com/stretchr/testify/require"
)

type fakeDriver struct {
	repo   string
	branch string
	commit string
}

func (f *fakeDriver) CreateCredential(_ context.Context, repo, branch, baseCommit string) (git.Credential, error) {
	f.repo = repo
	f.branch = branch
	f.commit = baseCommit
	return git.Credential{Token: "tok", RepoURL: "https://example.com/" + repo, Branch: branch, BaseCommit: baseCommit, ExpiresAt: time.Now()}, nil
}

func (f *fakeDriver) GetCommit(context.Context, string, string) (git.Commit, error)          { return git.Commit{}, nil }
func (f *fakeDriver) CompareCommits(context.Context, string, string, string) ([]git.ChangedFile, error) {
	return nil, nil
}
func (f *fakeDriver) IsAncestor(context.Context, string, string, string) (bool, error) { return false, nil }

func TestIssuerDelegatesToDriver(t *testing.T) {
	d := &fakeDriver{}
	issuer := git.NewIssuer(d)

	cred, err := issuer.Issue(context.Background(), "tenant-1", "exec-1", "owner/repo", "main", "abc")
	require.NoError(t, err)
	require.Equal(t, "tok", cred.Token)
	require.Equal(t, "owner/repo", d.repo)
	require.Equal(t, "main", d.branch)
	require.Equal(t, "abc", d.commit)
}
