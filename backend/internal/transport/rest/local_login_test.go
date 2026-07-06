package rest_test

import (
	"net/http"
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/config"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestLocalLoginSuccessSetsSessionCookie(t *testing.T) {
	cfg := config.Config{
		SessionCookieSecret: testSessionSecret,
		SessionCookieSecure: false,
		LocalAdmin: struct {
			TenantID   string
			OwnerID    string
			OwnerEmail string
			Password   string
			Enabled    bool
		}{
			TenantID:   "local-tenant",
			OwnerID:    "local-owner",
			OwnerEmail: "admin@local",
			Password:   "correct-password-1234",
			Enabled:    true,
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithLocalAdmin(cfg))

	res := postJSONNoAuth(t, server, "/oauth/local/login", `{"password":"correct-password-1234"}`)

	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"authenticated":true`)

	cookie := findCookie(res.Result().Cookies(), auth.SessionCookieName)
	require.NotNil(t, cookie, "expected session cookie to be set")
	require.True(t, cookie.HttpOnly)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, "/", cookie.Path)

	session, err := auth.ParseSessionCookie(cookie, testSessionSecret)
	require.NoError(t, err)
	require.Equal(t, "local-tenant", session.TenantID)
	require.Equal(t, "local-owner", session.OwnerID)
	require.Equal(t, "admin@local", session.OwnerEmail)
	require.True(t, session.IsAdmin)
}

func TestLocalLoginWrongPasswordReturns401(t *testing.T) {
	cfg := config.Config{
		SessionCookieSecret: testSessionSecret,
		SessionCookieSecure: false,
		LocalAdmin: struct {
			TenantID   string
			OwnerID    string
			OwnerEmail string
			Password   string
			Enabled    bool
		}{
			TenantID:   "local-tenant",
			OwnerID:    "local-owner",
			OwnerEmail: "admin@local",
			Password:   "correct-password-1234",
			Enabled:    true,
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithLocalAdmin(cfg))

	res := postJSONNoAuth(t, server, "/oauth/local/login", `{"password":"wrong-password"}`)

	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.Contains(t, res.Body.String(), "invalid password")
	require.Nil(t, findCookie(res.Result().Cookies(), auth.SessionCookieName))
}

func TestLocalLoginDisabledRouteReturns404(t *testing.T) {
	cfg := config.Config{
		SessionCookieSecret: testSessionSecret,
		SessionCookieSecure: false,
		LocalAdmin: struct {
			TenantID   string
			OwnerID    string
			OwnerEmail string
			Password   string
			Enabled    bool
		}{
			TenantID:   "local-tenant",
			OwnerID:    "local-owner",
			OwnerEmail: "admin@local",
			Password:   "correct-password-1234",
			Enabled:    false,
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithLocalAdmin(cfg))

	res := postJSONNoAuth(t, server, "/oauth/local/login", `{"password":"correct-password-1234"}`)

	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestLocalLoginWithoutOptionRouteReturns404(t *testing.T) {
	server := newTestServer(&fakeApplication{})

	res := postJSONNoAuth(t, server, "/oauth/local/login", `{"password":"correct-password-1234"}`)

	require.Equal(t, http.StatusNotFound, res.Code)
}
