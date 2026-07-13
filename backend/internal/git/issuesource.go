package git

import (
	"context"
	"time"
)

// Repository is a repository visible to a GitHub App installation.
type Repository struct {
	FullName      string
	DefaultBranch string
	Visibility    string
}

// Issue is a GitHub issue projected to the fields the sync engine needs.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string // "open" | "closed"
	Labels    []string
	UpdatedAt time.Time
	HTMLURL   string
}

// IssueFilter selects which issues to list.
type IssueFilter struct {
	State  string // "open" | "closed" | "all"
	Labels []string
}

// IssueSource reads repositories and issues for a tenant's installation.
type IssueSource interface {
	ListInstallationRepositories(ctx context.Context) ([]Repository, error)
	ListIssues(ctx context.Context, repo string, filter IssueFilter, since time.Time) ([]Issue, error)
}

// ResolvedIssueSource couples an issue source with the canonical
// owner/repository identity selected by the repository inventory.
type ResolvedIssueSource struct {
	Source   IssueSource
	FullName string
}
