// Package git defines platform-agnostic abstractions for Git operations.
package git

import (
	"context"
	"time"
)

// Credential is a short-lived hosting-provider credential. The Branch and
// BaseCommit fields describe the requested operation but are not assumed to be
// enforced by the provider token itself. It must never be persisted in plaintext.
type Credential struct {
	Token      string
	RepoURL    string
	Branch     string
	BaseCommit string
	ExpiresAt  time.Time
}

// Commit represents a single commit in a Git repository.
type Commit struct {
	SHA         string
	Message     string
	Author      string
	Email       string
	CommittedAt time.Time
	URL         string
}

// ChangedFile represents one file entry in a comparison between two commits.
type ChangedFile struct {
	Filename  string
	Status    string // added, removed, modified, renamed
	Additions int
	Deletions int
	Patch     string
}

// Driver abstracts interactions with a Git hosting platform.
type Driver interface {
	CreateCredential(ctx context.Context, repo, branch, baseCommit string) (Credential, error)
	GetCommit(ctx context.Context, repo, sha string) (Commit, error)
	CompareCommits(ctx context.Context, repo, base, head string) ([]ChangedFile, error)
	IsAncestor(ctx context.Context, repo, base, head string) (bool, error)
}

// ResolvedDriver couples a repository-scoped driver with the canonical
// owner/repository identity selected by the repository inventory.
type ResolvedDriver struct {
	Driver   Driver
	FullName string
}
