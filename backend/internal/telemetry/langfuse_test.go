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
		require.Contains(t, r.URL.Query().Get("traceTags"), "execution:exe-1")

		resp := map[string]any{
			"data": []map[string]any{{"totalCost": 0.00123}},
			"meta": map[string]any{"cursor": "cursor-1"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := telemetry.NewLangfuseProvider(telemetry.LangfuseConfig{
		BaseURL:   server.URL,
		PublicKey: "public",
		SecretKey: "secret",
		Mode:      "cloud",
	}, server.Client())

	obs, err := p.Observe(context.Background(), ref)
	require.NoError(t, err)
	require.Equal(t, telemetry.CoverageFull, obs.Coverage)
	require.True(t, obs.ObservedCost.GreaterThan(decimal.Zero))
	require.Equal(t, "langfuse", obs.Provider)
	require.Equal(t, "cursor-1", obs.Cursor)
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
	require.NoError(t, err)
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
