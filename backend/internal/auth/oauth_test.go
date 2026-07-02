package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestJWKSVerifierAcceptsValidToken(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	principal, err := verifier.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "tenant-1", principal.TenantID)
	require.Equal(t, "agent-1", principal.AgentID)
	require.Equal(t, "version-1", principal.AgentVersionID)
	require.Equal(t, []string{"tasks:read"}, principal.Scopes)
}

func TestJWKSVerifierAcceptsSpaceSeparatedScopes(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           "tasks:read tasks:write",
	}, time.Hour)

	principal, err := verifier.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, []string{"tasks:read", "tasks:write"}, principal.Scopes)
}

func TestJWKSVerifierRejectsTokenSignedByWrongKey(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid1, _, jwks1 := newRSAJWKS(t)
	_, key2, _ := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks1)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid1, key2, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	_, err := verifier.Verify(context.Background(), token)
	require.Error(t, err)
}

func TestJWKSVerifierRejectsTamperedToken(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	tampered := strings.ReplaceAll(string(payload), `"agent_id":"agent-1"`, `"agent_id":"attacker"`)
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(tampered))
	token = strings.Join(parts, ".")

	_, err = verifier.Verify(context.Background(), token)
	require.Error(t, err)
}

func TestJWKSVerifierRejectsExpiredToken(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, -time.Hour)

	_, err := verifier.Verify(context.Background(), token)
	require.Error(t, err)
}

func TestJWKSVerifierRejectsWrongIssuer(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, "https://evil.example.com", audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	_, err := verifier.Verify(context.Background(), token)
	require.Error(t, err)
}

func TestJWKSVerifierRejectsWrongAudience(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, "other-aud", map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	_, err := verifier.Verify(context.Background(), token)
	require.Error(t, err)
}

func TestJWKSVerifierRejectsMissingIdentityClaims(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	for name, claims := range map[string]map[string]any{
		"tenant": {
			"agent_id":         "agent-1",
			"agent_version_id": "version-1",
			"scopes":           []string{"tasks:read"},
		},
		"agent": {
			"tenant_id":        "tenant-1",
			"agent_version_id": "version-1",
			"scopes":           []string{"tasks:read"},
		},
		"version": {
			"tenant_id": "tenant-1",
			"agent_id":  "agent-1",
			"scopes":    []string{"tasks:read"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			token := signedToken(t, kid, key, issuer, audience, claims, time.Hour)
			_, err := verifier.Verify(context.Background(), token)
			require.Error(t, err)
		})
	}
}

func TestJWKSVerifierCachesKeysAndHandlesRotation(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid1, key1, jwks1 := newRSAJWKS(t)
	kid2, key2, jwks2 := newRSAJWKS(t)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(jwks1)
			return
		}
		_, _ = w.Write(jwks2)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token1 := signedToken(t, kid1, key1, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	_, err := verifier.Verify(context.Background(), token1)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// 用旧 key 再次验证应命中缓存，不额外请求。
	_, err = verifier.Verify(context.Background(), token1)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// 用新 key 验证应触发重新获取 JWKS。
	token2 := signedToken(t, kid2, key2, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)
	_, err = verifier.Verify(context.Background(), token2)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestJWKSVerifierRetainsOldKeyDuringRotation(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid1, key1, jwks1 := newRSAJWKS(t)
	kid2, key2, jwks2 := newRSAJWKS(t)

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write(jwks1)
			return
		}
		_, _ = w.Write(jwks2)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token1 := signedToken(t, kid1, key1, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	_, err := verifier.Verify(context.Background(), token1)
	require.NoError(t, err)
	require.Equal(t, 1, calls)

	// 触发一次只返回新 key 的刷新。
	token2 := signedToken(t, kid2, key2, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)
	_, err = verifier.Verify(context.Background(), token2)
	require.NoError(t, err)
	require.Equal(t, 2, calls)

	// 用旧 key 签名的 token 仍应通过，说明轮换过渡期旧 key 未被删除。
	_, err = verifier.Verify(context.Background(), token1)
	require.NoError(t, err)
}

func TestJWKSVerifierFetchesKeysOnDemand(t *testing.T) {
	issuer := "https://auth.example.com"
	audience := "agentguild"
	kid, key, jwks := newRSAJWKS(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jwks)
	}))
	defer server.Close()

	verifier := auth.NewJWKSVerifier(issuer, audience, server.URL, nil)
	token := signedToken(t, kid, key, issuer, audience, map[string]any{
		"tenant_id":        "tenant-1",
		"agent_id":         "agent-1",
		"agent_version_id": "version-1",
		"scopes":           []string{"tasks:read"},
	}, time.Hour)

	principal, err := verifier.Verify(context.Background(), token)
	require.NoError(t, err)
	require.Equal(t, "agent-1", principal.AgentID)
}

// newRSAJWKS 生成一个 RSA key pair 并返回 kid、私钥与 JWKS JSON。每次调用使用不同 kid。
func newRSAJWKS(t *testing.T) (string, *rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pub := key.Public().(*rsa.PublicKey)
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	kid := fmt.Sprintf("key-%d", time.Now().UnixNano())
	jwks := []byte(fmt.Sprintf(`{"keys":[{"kty":"RSA","kid":"%s","use":"sig","n":"%s","e":"%s"}]}`, kid, n, e))
	return kid, key, jwks
}

func signedToken(t *testing.T, kid string, key *rsa.PrivateKey, issuer, audience string, custom map[string]any, ttl time.Duration) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss": issuer,
		"aud": audience,
		"sub": "agent-1",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(ttl).Unix(),
	}
	for k, v := range custom {
		claims[k] = v
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func TestTokenVerifierInterfaceContract(t *testing.T) {
	var verifier auth.TokenVerifier = stubVerifierFunc(func(context.Context, string) (auth.Principal, error) {
		return auth.Principal{TenantID: "tenant-1"}, nil
	})
	principal, err := verifier.Verify(context.Background(), "token")
	require.NoError(t, err)
	require.Equal(t, "tenant-1", principal.TenantID)
}

type stubVerifierFunc func(context.Context, string) (auth.Principal, error)

func (fn stubVerifierFunc) Verify(ctx context.Context, token string) (auth.Principal, error) {
	return fn(ctx, token)
}
