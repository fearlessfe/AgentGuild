package telemetry_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentguild.dev/agentguild/backend/internal/telemetry"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestFailingProviderReturnsError(t *testing.T) {
	want := errors.New("timeout")
	p := telemetry.FailingProvider(want)
	_, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.ErrorIs(t, err, want)
}

func TestLangfuseCloudProviderReturnsFullCost(t *testing.T) {
	ref := telemetry.ExecutionRef{TenantID: "tenant-1", TaskID: "task-1", ExecutionID: "exe-1", AgentVersionID: "agent-1"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/public/v2/metrics", r.URL.Path)
		user, pass, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "public", user)
		require.Equal(t, "secret", pass)
		var query struct {
			View          string `json:"view"`
			FromTimestamp string `json:"fromTimestamp"`
			ToTimestamp   string `json:"toTimestamp"`
			Metrics       []struct {
				Measure     string `json:"measure"`
				Aggregation string `json:"aggregation"`
			} `json:"metrics"`
			Filters []struct {
				Column   string   `json:"column"`
				Operator string   `json:"operator"`
				Value    []string `json:"value"`
				Type     string   `json:"type"`
			} `json:"filters"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.URL.Query().Get("query")), &query))
		require.Equal(t, "observations", query.View)
		require.NotEmpty(t, query.FromTimestamp)
		require.NotEmpty(t, query.ToTimestamp)
		require.Equal(t, "totalCost", query.Metrics[0].Measure)
		require.Equal(t, "sum", query.Metrics[0].Aggregation)
		require.Len(t, query.Filters, 1)
		require.Equal(t, "traceTags", query.Filters[0].Column)
		require.Equal(t, "all of", query.Filters[0].Operator)
		require.Equal(t, "arrayOptions", query.Filters[0].Type)
		require.Equal(t, []string{"tenant:tenant-1", "task:task-1", "execution:exe-1", "agent_version:agent-1", "cost_coverage:complete"}, query.Filters[0].Value)

		resp := map[string]any{
			"data": []map[string]any{{"sum_totalCost": "0.00123"}},
			"meta": map[string]any{"cursor": "cursor-1"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:             server.URL,
		PublicKey:           "public",
		SecretKey:           "secret",
		Mode:                "cloud",
		CompleteCoverageTag: "cost_coverage:complete",
	}, server.Client())

	obs, err := p.Observe(context.Background(), ref)
	require.NoError(t, err)
	require.Equal(t, telemetry.CoverageComplete, obs.Coverage)
	require.True(t, obs.ObservedCost.GreaterThan(decimal.Zero))
	require.Equal(t, "langfuse", obs.Provider)
	require.Equal(t, "cursor-1", obs.Cursor)
}

func TestLangfuseSelfHostedUsesConfiguredCompatibleMetricsPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/custom/metrics", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"sum_totalCost": "1.25"}}})
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:             server.URL,
		Mode:                "self-hosted",
		SupportsCost:        true,
		MetricsPath:         "/custom/metrics",
		CompleteCoverageTag: "cost_coverage:complete",
	}, server.Client())
	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.NoError(t, err)
	require.Equal(t, telemetry.CoverageComplete, obs.Coverage)
	require.Equal(t, "1.25", obs.ObservedCost.String())
}

func TestLangfuseCostWithoutExplicitCoverageEvidenceRemainsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"sum_totalCost": "1.25"}}})
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{BaseURL: server.URL, Mode: "cloud"}, server.Client())
	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{ExecutionID: "execution"})
	require.NoError(t, err)
	require.Equal(t, telemetry.CoveragePartial, obs.Coverage)
}

func TestLangfuseSelfHostedWithoutCostSupportReturnsUnavailable(t *testing.T) {
	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		Mode:         "self-hosted",
		SupportsCost: false,
	}, nil)
	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.NoError(t, err)
	require.Equal(t, telemetry.CoverageUnavailable, obs.Coverage)
}

func TestLangfuseMalformedCostReturnsUnavailableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"sum_totalCost":"not-a-decimal"}]}`))
	}))
	defer server.Close()
	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{BaseURL: server.URL, Mode: "cloud"}, server.Client())
	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.Error(t, err)
	require.Equal(t, telemetry.CoverageUnavailable, obs.Coverage)
}

func TestLangfuseCloudHTTPErrorReturnsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:   server.URL,
		PublicKey: "public",
		SecretKey: "secret",
		Mode:      "cloud",
	}, server.Client())

	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.Error(t, err)
	require.Equal(t, telemetry.CoverageUnavailable, obs.Coverage)
}

func TestLangfuseCloudNoCostReturnsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}, "meta": map[string]any{"cursor": "cursor-partial"}})
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:   server.URL,
		PublicKey: "public",
		SecretKey: "secret",
		Mode:      "cloud",
	}, server.Client())

	obs, err := p.Observe(context.Background(), telemetry.ExecutionRef{})
	require.NoError(t, err)
	require.Equal(t, telemetry.CoveragePartial, obs.Coverage)
	require.Equal(t, "cursor-partial", obs.Cursor)
}

func TestLangfuseProviderIncludesBasicAuth(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{}})
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:   server.URL,
		PublicKey: "pk",
		SecretKey: "sk",
		Mode:      "cloud",
	}, server.Client())
	_, _ = p.Observe(context.Background(), telemetry.ExecutionRef{})

	require.NotEmpty(t, gotAuth)
	encoded := base64.StdEncoding.EncodeToString([]byte("pk:sk"))
	require.Equal(t, "Basic "+encoded, gotAuth)
}
