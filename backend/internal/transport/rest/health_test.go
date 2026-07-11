package rest_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	rest "agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

type healthChecker struct {
	err error
}

func (h healthChecker) Ping(context.Context) error { return h.err }

func TestHealthzReturnsHealthyWhenDatabaseResponds(t *testing.T) {
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}, rest.WithHealthChecker(healthChecker{})).Router()
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"status":"healthy"}`, recorder.Body.String())
}

func TestHealthzReturnsUnhealthyWhenDatabaseDoesNotRespond(t *testing.T) {
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}, rest.WithHealthChecker(healthChecker{err: errors.New("database unavailable")})).Router()
	recorder := httptest.NewRecorder()

	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"status":"unhealthy"}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "database unavailable")
}
