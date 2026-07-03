package auth_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestOIDCProviderBeginAuthURLIncludesStateAndScopes(t *testing.T) {
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      "https://issuer.example.com/oauth/authorize",
		TokenURL:     "https://issuer.example.com/oauth/token",
		Scopes:       []string{"openid", "email", "profile"},
	}, fakeOIDCExchanger{})
	require.NoError(t, err)

	got := provider.BeginAuthURL("state-1")

	require.True(t, strings.HasPrefix(got, "https://issuer.example.com/oauth/authorize?"))
	require.Contains(t, got, "client_id=client-1")
	require.Contains(t, got, "redirect_uri=https%3A%2F%2Fapp.example.com%2Foauth%2Fcallback")
	require.Contains(t, got, "response_type=code")
	require.Contains(t, got, "scope=openid+email+profile")
	require.Contains(t, got, "state=state-1")
}

func TestOIDCProviderExchangeCreatesSessionWithAdminClaim(t *testing.T) {
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      "https://issuer.example.com/oauth/authorize",
		TokenURL:     "https://issuer.example.com/oauth/token",
		AdminClaim:   "agentguild_admin",
	}, fakeOIDCExchanger{claims: auth.OIDCClaims{
		"sub":              "owner-1",
		"email":            "owner@example.com",
		"agentguild_admin": true,
	}})
	require.NoError(t, err)

	sess, err := provider.Exchange(context.Background(), "code-1")

	require.NoError(t, err)
	require.Equal(t, "tenant-1", sess.TenantID)
	require.Equal(t, "owner-1", sess.OwnerID)
	require.Equal(t, "owner@example.com", sess.OwnerEmail)
	require.True(t, sess.IsAdmin)
}

func TestOIDCProviderExchangeCreatesAdminSessionForConfiguredEmail(t *testing.T) {
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      "https://issuer.example.com/oauth/authorize",
		TokenURL:     "https://issuer.example.com/oauth/token",
		AdminEmails:  []string{"admin@example.com"},
	}, fakeOIDCExchanger{claims: auth.OIDCClaims{
		"sub":   "owner-1",
		"email": "ADMIN@example.com",
	}})
	require.NoError(t, err)

	sess, err := provider.Exchange(context.Background(), "code-1")

	require.NoError(t, err)
	require.True(t, sess.IsAdmin)
}

func TestOIDCProviderExchangeRejectsMissingIdentityClaims(t *testing.T) {
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      "https://issuer.example.com/oauth/authorize",
		TokenURL:     "https://issuer.example.com/oauth/token",
	}, fakeOIDCExchanger{claims: auth.OIDCClaims{"email": "owner@example.com"}})
	require.NoError(t, err)

	_, err = provider.Exchange(context.Background(), "code-1")

	require.Error(t, err)
}

func TestSessionCookieUsesSecureHTTPOnlyAttributes(t *testing.T) {
	session := auth.Session{
		TenantID:   "tenant-1",
		OwnerID:    "owner-1",
		OwnerEmail: "owner@example.com",
		IsAdmin:    true,
		ExpiresAt:  time.Now().Add(time.Hour),
	}

	cookie, err := auth.NewSessionCookie(session, "secret-key-with-at-least-32-bytes", true)

	require.NoError(t, err)
	require.Equal(t, "agentguild_session", cookie.Name)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
	require.Equal(t, "/", cookie.Path)
	require.NotContains(t, cookie.Value, "owner@example.com")

	got, err := auth.ParseSessionCookie(cookie, "secret-key-with-at-least-32-bytes")
	require.NoError(t, err)
	require.Equal(t, session.TenantID, got.TenantID)
	require.Equal(t, session.OwnerID, got.OwnerID)
	require.Equal(t, session.OwnerEmail, got.OwnerEmail)
	require.Equal(t, session.IsAdmin, got.IsAdmin)
}

func TestSessionCookieRejectsTampering(t *testing.T) {
	cookie, err := auth.NewSessionCookie(auth.Session{
		TenantID:   "tenant-1",
		OwnerID:    "owner-1",
		OwnerEmail: "owner@example.com",
	}, "secret-key-with-at-least-32-bytes", true)
	require.NoError(t, err)
	cookie.Value += "tampered"

	_, err = auth.ParseSessionCookie(cookie, "secret-key-with-at-least-32-bytes")

	require.Error(t, err)
}

type fakeOIDCExchanger struct {
	claims auth.OIDCClaims
	err    error
}

func (f fakeOIDCExchanger) ExchangeOIDC(context.Context, string) (auth.OIDCClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.claims == nil {
		return nil, errors.New("missing fake claims")
	}
	return f.claims, nil
}
