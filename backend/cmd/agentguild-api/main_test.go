package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	require.True(t, observation.Disabled)
}

func TestShutdownJoinsWorkersBeforePoolClose(t *testing.T) {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	var workerRan atomic.Bool
	runWorker(ctx, &wg, time.Millisecond, "test", func(ctx context.Context) error {
		workerRan.Store(true)
		return nil
	})
	time.Sleep(5 * time.Millisecond)
	require.True(t, workerRan.Load(), "worker should have run")
	cancel()
	wg.Wait()
}
