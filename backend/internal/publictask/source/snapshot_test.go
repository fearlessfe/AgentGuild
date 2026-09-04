package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitClonerRejectsUntrustedRepositoryAndCommit(t *testing.T) {
	cloner := NewGitCloner(nil, 10, 1024)
	_, err := cloner.Snapshot(context.Background(), "file:///tmp/repo", "main")
	require.ErrorContains(t, err, "owner/name")
	_, err = cloner.Snapshot(context.Background(), "owner/repo", "main")
	require.ErrorContains(t, err, "immutable")
}

func TestSnapshotFiltersGeneratedAndCredentialFiles(t *testing.T) {
	require.True(t, ignoredDirectory("node_modules/pkg"))
	require.True(t, ignoredDirectory(".git"))
	require.True(t, ignoredFile(".env.production"))
	require.True(t, ignoredFile("config/credentials.json"))
	require.True(t, textExtension("main.go"))
	require.True(t, textExtension("Dockerfile"))
	require.False(t, textExtension("image.png"))
}
