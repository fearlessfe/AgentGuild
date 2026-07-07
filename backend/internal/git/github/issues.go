package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

const issuesPerPage = 100

type repositoryPayload struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
	Private       bool   `json:"private"`
}

type issuePayload struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	HTMLURL   string `json:"html_url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct {
		URL string `json:"url"`
	} `json:"pull_request"`
}

// ListInstallationRepositories returns repositories visible to this App installation.
func (d *Driver) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	token, _, err := d.installationToken(ctx)
	if err != nil {
		return nil, err
	}

	var repos []git.Repository
	for page := 1; ; page++ {
		u := d.apiURL("/installation/repositories?per_page=%d&page=%d", issuesPerPage, page)
		body, status, retryAfter, err := d.get(ctx, token, u)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, mapError(status, body, retryAfter)
		}

		var payload struct {
			Repositories []repositoryPayload `json:"repositories"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode repositories: %w", err)
		}
		if len(payload.Repositories) == 0 {
			break
		}

		for _, repo := range payload.Repositories {
			repos = append(repos, git.Repository{
				FullName:      repo.FullName,
				DefaultBranch: repo.DefaultBranch,
				Visibility:    repositoryVisibility(repo),
			})
		}
		if len(payload.Repositories) < issuesPerPage {
			break
		}
	}
	return repos, nil
}

// ListIssues returns GitHub issues for repo, excluding pull requests returned by the issues API.
func (d *Driver) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	token, _, err := d.installationToken(ctx)
	if err != nil {
		return nil, err
	}

	state := filter.State
	if state == "" {
		state = "open"
	}

	var issues []git.Issue
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("state", state)
		q.Set("per_page", fmt.Sprintf("%d", issuesPerPage))
		q.Set("page", fmt.Sprintf("%d", page))
		if len(filter.Labels) > 0 {
			q.Set("labels", strings.Join(filter.Labels, ","))
		}
		if !since.IsZero() {
			q.Set("since", since.UTC().Format(time.RFC3339))
		}

		u := d.apiURL("/repos/%s/%s/issues?%s", owner, name, q.Encode())
		body, status, retryAfter, err := d.get(ctx, token, u)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, mapError(status, body, retryAfter)
		}

		var payload []issuePayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode issues: %w", err)
		}
		if len(payload) == 0 {
			break
		}

		for _, item := range payload {
			if item.PullRequest != nil {
				continue
			}
			labels := make([]string, 0, len(item.Labels))
			for _, label := range item.Labels {
				labels = append(labels, label.Name)
			}
			updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
			issues = append(issues, git.Issue{
				Number:    item.Number,
				Title:     item.Title,
				Body:      item.Body,
				State:     item.State,
				Labels:    labels,
				UpdatedAt: updatedAt,
				HTMLURL:   item.HTMLURL,
			})
		}
		if len(payload) < issuesPerPage {
			break
		}
	}
	return issues, nil
}

func repositoryVisibility(repo repositoryPayload) string {
	if repo.Visibility != "" {
		return repo.Visibility
	}
	if repo.Private {
		return "private"
	}
	return "public"
}

var _ git.IssueSource = (*Driver)(nil)
