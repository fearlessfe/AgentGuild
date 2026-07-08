package github_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gh "agentguild.dev/agentguild/backend/internal/git/github"
	"github.com/stretchr/testify/require"
)

func TestPublicIssueSourceListsPublicIssuesWithoutAuthorization(t *testing.T) {
	var gotAuth string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/repos/owner/repo/issues", r.URL.Path)
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		fmt.Fprint(w, `[{"number":1,"title":"ready","body":"ship it","state":"open","labels":[{"name":"agent-ready"}],"updated_at":"2026-07-01T00:00:00Z","html_url":"https://github.com/owner/repo/issues/1"},{"number":2,"title":"skip pr","state":"open","pull_request":{"url":"https://api.github.com/repos/owner/repo/pulls/2"}}]`)
	}))
	defer srv.Close()

	source := gh.NewPublicIssueSource(srv.URL, srv.Client())
	since := time.Date(2026, 7, 1, 1, 2, 3, 0, time.FixedZone("CST", 8*60*60))
	issues, err := source.ListIssues(context.Background(), "owner/repo", git.IssueFilter{
		State:  "open",
		Labels: []string{"agent-ready"},
	}, since)
	require.NoError(t, err)

	require.Empty(t, gotAuth)
	require.Equal(t, "open", gotQuery.Get("state"))
	require.Equal(t, "agent-ready", gotQuery.Get("labels"))
	require.Equal(t, "2026-06-30T17:02:03Z", gotQuery.Get("since"))
	require.Len(t, issues, 1)
	require.Equal(t, git.Issue{
		Number:    1,
		Title:     "ready",
		Body:      "ship it",
		State:     "open",
		Labels:    []string{"agent-ready"},
		UpdatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		HTMLURL:   "https://github.com/owner/repo/issues/1",
	}, issues[0])
}
