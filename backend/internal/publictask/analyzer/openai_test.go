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

func TestOpenAIAnalyzerUsesCompatibleEndpointAndParsesContract(t *testing.T) {
	var got http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		require.Equal(t, "Bearer test-openai-key", r.Header.Get("authorization"))
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		analysis := `{"title":"Fix retries","summary":"Retry safely","problem_diagnosis":"The loop retries non-idempotent calls.","impact":"Duplicate writes.","proposed_solution":"Guard retries by method.","implementation_steps":["update code"],"constraints":["keep API"],"non_goals":["redesign"],"risks":["compatibility"],"acceptance_criteria":[{"id":"test","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"exit 0"}]}`
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, analysis)
	}))
	defer server.Close()

	analyzer, err := publictaskanalyzer.NewOpenAI("test-openai-key", "gpt-5.6-sol", server.URL, server.Client())
	require.NoError(t, err)
	result, err := analyzer.Analyze(context.Background(), publictaskanalysis.Input{
		Repository: "owner/repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567",
		IssueURL: "https://github.com/owner/repo/issues/7", IssueNumber: 7,
		Title: "Retry bug", Problem: "Retries duplicate writes.",
	})
	require.NoError(t, err)
	require.Equal(t, "Fix retries", result.Title)
	require.Equal(t, http.MethodPost, got.Method)
}

func TestOpenAIAnalyzerRejectsIncompleteContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"only title\"}"}}]}`)
	}))
	defer server.Close()
	analyzer, err := publictaskanalyzer.NewOpenAI("test-key", "test-model", server.URL, server.Client())
	require.NoError(t, err)
	_, err = analyzer.Analyze(context.Background(), publictaskanalysis.Input{Repository: "owner/repo", BaseCommit: "0123456789abcdef0123456789abcdef01234567"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "analyzer result missing")
}
