package auth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"github.com/stretchr/testify/require"
)

func TestOIDCProviderBeginAuthURLIncludesStateAndScopes(t *testing.T) {
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		Issuer:       "https://issuer.example.com",
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
		Issuer:       "https://issuer.example.com",
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
		Issuer:       "https://issuer.example.com",
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
		Issuer:       "https://issuer.example.com",
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

func TestNewOIDCProviderRequiresIssuer(t *testing.T) {
	_, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:    "tenant-1",
		ClientID:    "client-1",
		RedirectURI: "https://app.example.com/oauth/callback",
		AuthURL:     "https://issuer.example.com/oauth/authorize",
		TokenURL:    "https://issuer.example.com/oauth/token",
	}, fakeOIDCExchanger{})

	require.Error(t, err)
}

func TestOIDCProviderDefaultExchangerCreatesSessionFromVerifiedIDToken(t *testing.T) {
	issuer := "https://issuer.example.com"
	clientID := "client-1"
	kid, key, jwks := newRSAJWKS(t)
	tokenServer := oidcTokenServer(t, jwks, signedToken(t, kid, key, issuer, clientID, map[string]any{
		"sub":   "owner-1",
		"email": "owner@example.com",
	}, time.Hour))
	defer tokenServer.Close()

	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      issuer + "/oauth/authorize",
		TokenURL:     tokenServer.URL + "/token",
		JWKSURL:      tokenServer.URL + "/jwks",
	}, nil)
	require.NoError(t, err)

	sess, err := provider.Exchange(context.Background(), "code-1")

	require.NoError(t, err)
	require.Equal(t, "owner-1", sess.OwnerID)
	require.Equal(t, "owner@example.com", sess.OwnerEmail)
}

func TestOIDCProviderDefaultExchangerRejectsUnsignedIDToken(t *testing.T) {
	issuer := "https://issuer.example.com"
	clientID := "client-1"
	_, _, jwks := newRSAJWKS(t)
	tokenServer := oidcTokenServer(t, jwks, unsignedIDToken(t, map[string]any{
		"iss":   issuer,
		"aud":   clientID,
		"sub":   "owner-1",
		"email": "owner@example.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}))
	defer tokenServer.Close()

	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      issuer + "/oauth/authorize",
		TokenURL:     tokenServer.URL + "/token",
		JWKSURL:      tokenServer.URL + "/jwks",
	}, nil)
	require.NoError(t, err)

	_, err = provider.Exchange(context.Background(), "code-1")

	require.Error(t, err)
}

func TestOIDCProviderDefaultExchangerRejectsInvalidSignatureIDToken(t *testing.T) {
	issuer := "https://issuer.example.com"
	clientID := "client-1"
	kid, _, jwks := newRSAJWKS(t)
	_, wrongKey, _ := newRSAJWKS(t)
	tokenServer := oidcTokenServer(t, jwks, signedToken(t, kid, wrongKey, issuer, clientID, map[string]any{
		"sub":   "owner-1",
		"email": "owner@example.com",
	}, time.Hour))
	defer tokenServer.Close()

	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		TenantID:     "tenant-1",
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: "secret-1",
		RedirectURI:  "https://app.example.com/oauth/callback",
		AuthURL:      issuer + "/oauth/authorize",
		TokenURL:     tokenServer.URL + "/token",
		JWKSURL:      tokenServer.URL + "/jwks",
	}, nil)
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

func unsignedIDToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func oidcTokenServer(t *testing.T, jwks []byte, idToken string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]string{
			"access_token": "access-1",
			"token_type":   "Bearer",
			"id_token":     idToken,
		})
		require.NoError(t, err)
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})
	return httptest.NewServer(mux)
}
