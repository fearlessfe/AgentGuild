package postgres_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestOpenRegistrationPersistsGlobalIdentityAndIssuesUsableToken(t *testing.T) {
	db := testdb.StartPostgres(t)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	issuer, err := auth.NewRS256TokenIssuer(privateKey, auth.TokenIssuerConfig{Issuer: "agentguild", Audience: "agentguild", TTL: 15 * time.Minute})
	require.NoError(t, err)
	service, err := identityapp.NewOpenRegistrationService(identitypostgres.NewOpenRegistrationStore(db), identityapp.OpenRegistrationOptions{
		TokenIssuer: issuer, OrganizationID: "public",
	})
	require.NoError(t, err)
	publicKey, privateAgentKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	challengeResult, err := service.CreateChallenge(context.Background(), identityapp.CreateRegistrationChallenge{PublicKey: publicKey})
	require.NoError(t, err)
	var nonce []byte
	require.NoError(t, db.QueryRow(context.Background(), `SELECT nonce FROM agent_registration_challenges WHERE id=$1`, challengeResult.Data.ChallengeID).Scan(&nonce))
	proof := ed25519.Sign(privateAgentKey, identitydomain.RegistrationProofMessage(challengeResult.Data.ChallengeID, nonce, publicKey))

	registrationCommand := identityapp.OpenRegisterAgent{
		ChallengeID: challengeResult.Data.ChallengeID, PublicKey: publicKey, Signature: proof,
		Handle: "postgres-worker", Runtime: "codex", Model: "gpt-5", Capabilities: []string{"shell"},
	}
	registration, err := service.Register(context.Background(), registrationCommand)
	require.NoError(t, err)
	require.NotEmpty(t, registration.Data.AgentID)
	require.Equal(t, "public", registration.Data.OrganizationID)

	var identityStatus, versionStatus, membershipStatus, challengeStatus string
	require.NoError(t, db.QueryRow(context.Background(), `SELECT status FROM agent_identities WHERE id=$1`, registration.Data.AgentID).Scan(&identityStatus))
	require.NoError(t, db.QueryRow(context.Background(), `SELECT status FROM agent_identity_versions WHERE id=$1`, registration.Data.AgentVersionID).Scan(&versionStatus))
	require.NoError(t, db.QueryRow(context.Background(), `SELECT status FROM agent_organization_memberships WHERE agent_id=$1 AND organization_id='public'`, registration.Data.AgentID).Scan(&membershipStatus))
	require.NoError(t, db.QueryRow(context.Background(), `SELECT status FROM agent_registration_challenges WHERE id=$1`, challengeResult.Data.ChallengeID).Scan(&challengeStatus))
	require.Equal(t, "active", identityStatus)
	require.Equal(t, "active", versionStatus)
	require.Equal(t, "active", membershipStatus)
	require.Equal(t, "consumed", challengeStatus)
	_, err = service.Register(context.Background(), registrationCommand)
	require.ErrorIs(t, err, identitydomain.ErrTokenExpired)

	verifier := auth.NewRS256Verifier(&privateKey.PublicKey, auth.TokenVerifierConfig{Issuer: "agentguild", Audience: "agentguild"})
	principal, err := verifier.Verify(context.Background(), registration.Data.AccessToken)
	require.NoError(t, err)
	require.True(t, principal.IsGlobalAgent())
	require.Equal(t, registration.Data.AgentID, principal.AgentID)
	require.Contains(t, principal.Scopes, "tasks:claim")
	refreshed, err := service.Refresh(context.Background(), principal.AgentID, principal.AgentVersionID)
	require.NoError(t, err)
	require.NotEmpty(t, refreshed.Data.Token)
	heartbeat, err := service.Heartbeat(context.Background(), principal.AgentID, principal.AgentVersionID)
	require.NoError(t, err)
	require.NotNil(t, heartbeat.Data.LastSeenAt)
	recoveryChallenge, err := service.CreateChallenge(context.Background(), identityapp.CreateRegistrationChallenge{PublicKey: publicKey})
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(context.Background(), `SELECT nonce FROM agent_registration_challenges WHERE id=$1`, recoveryChallenge.Data.ChallengeID).Scan(&nonce))
	recoveryProof := ed25519.Sign(privateAgentKey, identitydomain.RegistrationProofMessage(recoveryChallenge.Data.ChallengeID, nonce, publicKey))
	recovered, err := service.Register(context.Background(), identityapp.OpenRegisterAgent{
		ChallengeID: recoveryChallenge.Data.ChallengeID, PublicKey: publicKey, Signature: recoveryProof,
		Runtime: "codex", Model: "gpt-5",
	})
	require.NoError(t, err)
	require.Equal(t, registration.Data.AgentID, recovered.Data.AgentID)
}
