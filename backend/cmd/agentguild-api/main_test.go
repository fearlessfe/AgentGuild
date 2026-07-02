package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"agentguild.dev/agentguild/backend/internal/config"
	"agentguild.dev/agentguild/backend/internal/telemetry"
	"github.com/stretchr/testify/require"
)

func TestAdapterHandlerMountsOnlyEnabledTransports(t *testing.T) {
	web := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mcp := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) })
	handler := adapterHandler(true, false, web, mcp)

	restResponse := httptest.NewRecorder()
	handler.ServeHTTP(restResponse, httptest.NewRequest(http.MethodGet, "/v1/tasks", nil))
	require.Equal(t, http.StatusNoContent, restResponse.Code)

	mcpResponse := httptest.NewRecorder()
	handler.ServeHTTP(mcpResponse, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	require.Equal(t, http.StatusNotFound, mcpResponse.Code)
}

func TestDisabledLangfuseKeepsOutboxProviderAvailable(t *testing.T) {
	provider := costProvider(false, config.Config{})
	observation, err := provider.Observe(context.Background(), telemetry.ExecutionRef{ExecutionID: "execution-1"})
	require.NoError(t, err)
	require.Equal(t, telemetry.CoverageUnavailable, observation.Coverage)
}
