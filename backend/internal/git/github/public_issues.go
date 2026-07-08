package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

const defaultPublicBaseURL = "https://api.github.com"

type PublicIssueSource struct {
	baseURL string
	client  *http.Client
}

func NewPublicIssueSource(baseURL string, client *http.Client) *PublicIssueSource {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = defaultPublicBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &PublicIssueSource{baseURL: baseURL, client: client}
}

func (s *PublicIssueSource) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	return nil, nil
}

func (s *PublicIssueSource) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	owner, name, err := splitRepo(repo)
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

		body, status, retryAfter, err := s.get(ctx, s.apiURL("/repos/%s/%s/issues?%s", owner, name, q.Encode()))
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

		issues = append(issues, issuesFromPayload(payload)...)
		if len(payload) < issuesPerPage {
			break
		}
	}
	return issues, nil
}

func (s *PublicIssueSource) get(ctx context.Context, url string) ([]byte, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("request github api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header.Get("Retry-After"), err
	}
	return body, resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

func (s *PublicIssueSource) apiURL(path string, args ...any) string {
	return s.baseURL + fmt.Sprintf(path, args...)
}

func issuesFromPayload(payload []issuePayload) []git.Issue {
	issues := make([]git.Issue, 0, len(payload))
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
	return issues
}

var _ git.IssueSource = (*PublicIssueSource)(nil)
