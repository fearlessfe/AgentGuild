package analyzer_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	publictaskanalysis "agentguild.dev/agentguild/backend/internal/publictask/analysis"
	publictaskanalyzer "agentguild.dev/agentguild/backend/internal/publictask/analyzer"
	"github.com/stretchr/testify/require"
)

func TestAnthropicAnalyzerSendsPinnedContextAndParsesContract(t *testing.T) {
	var got http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		require.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		require.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		fmt.Fprint(w, `{"content":[{"type":"text","text":"{\"title\":\"Fix retries\",\"summary\":\"Retry safely\",\"problem_diagnosis\":\"The loop retries non-idempotent calls.\",\"impact\":\"Duplicate writes.\",\"proposed_solution\":\"Guard retries by method.\",\"implementation_steps\":[\"update code\"],\"constraints\":[\"keep API\"],\"non_goals\":[\"redesign\"],\"risks\":[\"compatibility\"],\"acceptance_criteria\":[{\"id\":\"test\",\"statement\":\"tests pass\",\"critical\":true,\"verifier_kind\":\"command\",\"expected_result\":\"exit 0\"}]}"}]}`)
	}))
	defer server.Close()

	analyzer, err := publictaskanalyzer.NewAnthropic("test-api-key", "test-model", server.URL, server.Client())
	require.NoError(t, err)
	result, err := analyzer.Analyze(context.Background(), publictaskanalysis.Input{
		Repository: "owner/repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		IssueURL: "https://github.com/owner/repo/issues/7", IssueNumber: 7,
		Title: "Retry bug", Problem: "Retries duplicate writes.",
		Files: []publictaskanalysis.SourceFile{{Path: "retry.go", Content: "func retry() {}"}},
	})
	require.NoError(t, err)
	require.Equal(t, "Fix retries", result.Title)
	require.Len(t, result.AcceptanceCriteria, 1)
	require.Equal(t, http.MethodPost, got.Method)
}

func TestAnthropicAnalyzerRejectsIncompleteContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"content":[{"type":"text","text":"{\"title\":\"only title\"}"}]}`)
	}))
	defer server.Close()
	analyzer, err := publictaskanalyzer.NewAnthropic("test-api-key", "test-model", server.URL, server.Client())
	require.NoError(t, err)
	_, err = analyzer.Analyze(context.Background(), publictaskanalysis.Input{Repository: "owner/repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analyzer result missing")
}
