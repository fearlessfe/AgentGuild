package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestTokenIssuerIssuesRS256AgentAccessToken(t *testing.T) {
	key := newTokenKey(t)
	issuer, err := auth.NewRS256TokenIssuer(key, auth.TokenIssuerConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
		KeyID:    "key-1",
	})
	require.NoError(t, err)
	agent, version := activeAgentAndVersion(t)
	now := time.Unix(1_700_000_000, 0)

	rawToken, err := issuer.Issue(agent, version, now)

	require.NoError(t, err)
	verifier := auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
		Now:      func() time.Time { return now.Add(time.Minute) },
	})
	principal, err := verifier.Verify(context.Background(), rawToken)
	require.NoError(t, err)
	require.Equal(t, auth.PrincipalTypeAgent, principal.Type)
	require.Equal(t, "tenant-1", principal.TenantID)
	require.Equal(t, "agent-1", principal.AgentID)
	require.Equal(t, "version-1", principal.AgentVersionID)
	require.Equal(t, []string{"tasks:read", "tasks:execute"}, principal.Scopes)
	require.Equal(t, []string{"acme/repo"}, principal.RepoScope)
}

func TestTokenIssuerUsesFifteenMinuteTTL(t *testing.T) {
	key := newTokenKey(t)
	issuer, err := auth.NewRS256TokenIssuer(key, auth.TokenIssuerConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
	})
	require.NoError(t, err)
	agent, version := activeAgentAndVersion(t)
	now := time.Unix(1_700_000_000, 0)
	rawToken, err := issuer.Issue(agent, version, now)
	require.NoError(t, err)

	verifier := auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
		Now:      func() time.Time { return now.Add(14*time.Minute + 59*time.Second) },
	})
	_, err = verifier.Verify(context.Background(), rawToken)
	require.NoError(t, err)

	expiredVerifier := auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
		Now:      func() time.Time { return now.Add(15*time.Minute + time.Second) },
	})
	_, err = expiredVerifier.Verify(context.Background(), rawToken)
	require.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestTokenVerifierRejectsExpiredAgentToken(t *testing.T) {
	key := newTokenKey(t)
	issuer, err := auth.NewRS256TokenIssuer(key, auth.TokenIssuerConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
	})
	require.NoError(t, err)
	agent, version := activeAgentAndVersion(t)
	now := time.Unix(1_700_000_000, 0)
	rawToken, err := issuer.Issue(agent, version, now)
	require.NoError(t, err)

	verifier := auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
		Now:      func() time.Time { return now.Add(16 * time.Minute) },
	})
	_, err = verifier.Verify(context.Background(), rawToken)

	require.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestTokenVerifierRejectsWrongSigningMethod(t *testing.T) {
	key := newTokenKey(t)
	agent, version := activeAgentAndVersion(t)
	rawToken := signedHS256AgentToken(t, "not-rsa", agent, version, time.Now())

	verifier := auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
		Issuer:   "agentguild",
		Audience: "agentguild-agents",
	})
	_, err := verifier.Verify(context.Background(), rawToken)

	require.Error(t, err)
	require.False(t, errors.Is(err, auth.ErrTokenExpired))
}

func TestTokenIssuerRejectsMismatchedAgentVersion(t *testing.T) {
	key := newTokenKey(t)
	issuer, err := auth.NewRS256TokenIssuer(key, auth.TokenIssuerConfig{})
	require.NoError(t, err)
	agent, version := activeAgentAndVersion(t)
	version.AgentID = "other-agent"

	_, err = issuer.Issue(agent, version, time.Now())

	require.Error(t, err)
}

func newTokenKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func activeAgentAndVersion(t *testing.T) (*domain.Agent, *domain.AgentVersion) {
	t.Helper()
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", []string{"tasks:read", "tasks:execute"}, nil)
	require.NoError(t, err)
	agent.Status = domain.AgentActive
	agent.RepoScope = []string{"acme/repo"}
	agent.CurrentVersionID = "version-1"
	version, err := domain.NewAgentVersion("version-1", "tenant-1", "agent-1", 1, "codex", "gpt-5", []string{"code"}, "fingerprint-1", time.Now())
	require.NoError(t, err)
	return agent, version
}

func signedHS256AgentToken(t *testing.T, secret string, agent *domain.Agent, version *domain.AgentVersion, now time.Time) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":              "agentguild",
		"aud":              "agentguild-agents",
		"tenant_id":        agent.TenantID,
		"agent_id":         agent.ID,
		"agent_version_id": version.ID,
		"scopes":           agent.Scopes,
		"iat":              now.Unix(),
		"exp":              now.Add(15 * time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}
