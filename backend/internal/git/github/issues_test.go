package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"github.com/stretchr/testify/require"
)

func TestListIssuesPaginatesAndFiltersPRs(t *testing.T) {
	key, pem := newRSAKey(t)
	page := 0
	var firstQuery url.Values
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/issues": func(w http.ResponseWriter, r *http.Request) {
			page++
			if page == 1 {
				firstQuery = r.URL.Query()
				fmt.Fprint(w, `[{"number":1,"title":"ready","body":"ship it","state":"open","labels":[{"name":"agent-ready"}],"updated_at":"2026-07-01T00:00:00Z","html_url":"https://github.example/owner/repo/issues/1"}`)
				for i := 2; i <= 100; i++ {
					fmt.Fprintf(w, `,{"number":%d,"title":"pr","state":"open","pull_request":{"url":"https://api.github.example/pulls/%d"}}`, i, i)
				}
				fmt.Fprint(w, `]`)
				return
			}
			fmt.Fprint(w, `[]`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	since := time.Date(2026, 7, 1, 1, 2, 3, 0, time.FixedZone("CST", 8*60*60))
	issues, err := d.ListIssues(context.Background(), "owner/repo", git.IssueFilter{
		State:  "open",
		Labels: []string{"agent-ready"},
	}, since)
	require.NoError(t, err)

	require.Equal(t, "open", firstQuery.Get("state"))
	require.Equal(t, "agent-ready", firstQuery.Get("labels"))
	require.Equal(t, "2026-06-30T17:02:03Z", firstQuery.Get("since"))
	require.Equal(t, "100", firstQuery.Get("per_page"))
	require.Equal(t, "1", firstQuery.Get("page"))
	require.Len(t, issues, 1)
	require.Equal(t, git.Issue{
		Number:    1,
		Title:     "ready",
		Body:      "ship it",
		State:     "open",
		Labels:    []string{"agent-ready"},
		UpdatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		HTMLURL:   "https://github.example/owner/repo/issues/1",
	}, issues[0])
	require.Equal(t, 2, page)
}

func TestListIssuesDefaultsToOpenState(t *testing.T) {
	key, pem := newRSAKey(t)
	var state string
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/repos/owner/repo/issues": func(w http.ResponseWriter, r *http.Request) {
			state = r.URL.Query().Get("state")
			fmt.Fprint(w, `[]`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	_, err := d.ListIssues(context.Background(), "owner/repo", git.IssueFilter{}, time.Time{})
	require.NoError(t, err)
	require.Equal(t, "open", state)
}

func TestListInstallationRepositoriesPaginates(t *testing.T) {
	key, pem := newRSAKey(t)
	page := 0
	var firstQuery url.Values
	srv := newGitHubServer(t, key, map[string]http.HandlerFunc{
		"/installation/repositories": func(w http.ResponseWriter, r *http.Request) {
			page++
			if page == 1 {
				firstQuery = r.URL.Query()
				fmt.Fprint(w, `{"repositories":[{"full_name":"owner/repo","default_branch":"main","visibility":"private"},{"full_name":"owner/public","default_branch":"trunk","private":false}`)
				for i := 3; i <= 100; i++ {
					fmt.Fprintf(w, `,{"full_name":"owner/repo-%d","default_branch":"main","private":true}`, i)
				}
				fmt.Fprint(w, `]}`)
				return
			}
			fmt.Fprint(w, `{"repositories":[]}`)
		},
	})
	defer srv.Close()

	d := newDriver(t, pem, srv.URL)
	repos, err := d.ListInstallationRepositories(context.Background())
	require.NoError(t, err)

	require.Equal(t, "100", firstQuery.Get("per_page"))
	require.Equal(t, "1", firstQuery.Get("page"))
	require.Len(t, repos, 100)
	require.Equal(t, git.Repository{FullName: "owner/repo", DefaultBranch: "main", Visibility: "private"}, repos[0])
	require.Equal(t, git.Repository{FullName: "owner/public", DefaultBranch: "trunk", Visibility: "public"}, repos[1])
	require.Equal(t, 2, page)
}
