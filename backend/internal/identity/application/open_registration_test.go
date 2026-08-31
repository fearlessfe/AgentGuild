package application_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/stretchr/testify/require"
)

func TestOpenRegistrationCreatesGlobalIdentityFromKeyProof(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	store := newOpenRegistrationMemoryStore(now)
	issuer := &openRegistrationIssuer{}
	service, err := application.NewOpenRegistrationService(store, application.OpenRegistrationOptions{
		NewID:          sequenceIDs("challenge-1", "agent-1", "version-1", "challenge-2"),
		TokenIssuer:    issuer,
		OrganizationID: "public",
	})
	require.NoError(t, err)

	challengeResult, err := service.CreateChallenge(context.Background(), application.CreateRegistrationChallenge{PublicKey: publicKey})
	require.NoError(t, err)
	challenge := store.challenge
	require.NotNil(t, challenge)

	proof := ed25519.Sign(privateKey, domain.RegistrationProofMessage(challenge.ID, challenge.Nonce, publicKey))
	result, err := service.Register(context.Background(), application.OpenRegisterAgent{
		ChallengeID: "challenge-1", PublicKey: publicKey, Signature: proof,
		Handle: "billing-worker", DisplayName: "Billing Worker", Runtime: "codex", Model: "gpt-5",
		Capabilities: []string{"shell"}, ConfigFingerprint: "sha256:config",
	})
	require.NoError(t, err)
	require.Equal(t, "agent-1", result.Data.AgentID)
	require.Equal(t, "version-1", result.Data.AgentVersionID)
	require.Equal(t, "public", result.Data.OrganizationID)
	require.Equal(t, "access-agent-1", result.Data.AccessToken)
	require.Equal(t, "active", store.identity.Status)
	require.Equal(t, "active", store.membership.Status)
	require.Equal(t, domain.RegistrationChallengeConsumed, store.challenge.Status)
	require.Len(t, store.events, 2)
	refreshed, err := service.Refresh(context.Background(), "agent-1", "version-1")
	require.NoError(t, err)
	require.Equal(t, "access-agent-1", refreshed.Data.Token)
	heartbeat, err := service.Heartbeat(context.Background(), "agent-1", "version-1")
	require.NoError(t, err)
	require.NotNil(t, heartbeat.Data.LastSeenAt)
	self, err := service.GetSelf(context.Background(), "agent-1", "version-1")
	require.NoError(t, err)
	require.Equal(t, "billing-worker", self.Data.Handle)

	_, err = service.Register(context.Background(), application.OpenRegisterAgent{
		ChallengeID: "challenge-1", PublicKey: publicKey, Signature: proof, Runtime: "codex", Model: "gpt-5",
	})
	require.ErrorIs(t, err, domain.ErrTokenExpired)
	recoveredChallenge, err := service.CreateChallenge(context.Background(), application.CreateRegistrationChallenge{PublicKey: publicKey})
	require.NoError(t, err)
	recoveryProof := ed25519.Sign(privateKey, domain.RegistrationProofMessage(store.challenge.ID, store.challenge.Nonce, publicKey))
	recovered, err := service.Register(context.Background(), application.OpenRegisterAgent{
		ChallengeID: recoveredChallenge.Data.ChallengeID, PublicKey: publicKey, Signature: recoveryProof, Runtime: "codex", Model: "gpt-5",
	})
	require.NoError(t, err)
	require.Equal(t, "agent-1", recovered.Data.AgentID)
	require.Equal(t, 3, issuer.calls)
	_ = challengeResult
}

func TestOpenRegistrationRejectsInvalidProofWithoutConsumingChallenge(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	store := newOpenRegistrationMemoryStore(time.Now().UTC())
	service, err := application.NewOpenRegistrationService(store, application.OpenRegistrationOptions{TokenIssuer: &openRegistrationIssuer{}})
	require.NoError(t, err)
	_, err = service.CreateChallenge(context.Background(), application.CreateRegistrationChallenge{PublicKey: publicKey})
	require.NoError(t, err)

	_, err = service.Register(context.Background(), application.OpenRegisterAgent{
		ChallengeID: store.challenge.ID, PublicKey: publicKey, Signature: make([]byte, ed25519.SignatureSize), Runtime: "codex", Model: "gpt-5",
	})
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.Equal(t, domain.RegistrationChallengePending, store.challenge.Status)
}

type openRegistrationIssuer struct{ calls int }

func (i *openRegistrationIssuer) IssueGlobal(agent *domain.AgentIdentity, _ *domain.AgentVersion, _ []string, _ time.Time) (string, error) {
	i.calls++
	return "access-" + agent.ID, nil
}

type openRegistrationMemoryStore struct {
	now        time.Time
	challenge  *domain.AgentRegistrationChallenge
	identity   *domain.AgentIdentity
	version    *domain.AgentVersion
	key        *domain.AgentIdentityKey
	membership *domain.OrganizationMembership
	events     []domain.AgentIdentityEvent
}

func newOpenRegistrationMemoryStore(now time.Time) *openRegistrationMemoryStore {
	return &openRegistrationMemoryStore{now: now}
}

func (s *openRegistrationMemoryStore) WithOpenRegistrationTx(_ context.Context, fn func(application.OpenRegistrationTx) error) error {
	return fn((*openRegistrationMemoryTx)(s))
}

type openRegistrationMemoryTx openRegistrationMemoryStore

func (tx *openRegistrationMemoryTx) Now(context.Context) (time.Time, error) { return tx.now, nil }
func (tx *openRegistrationMemoryTx) InsertChallenge(_ context.Context, challenge *domain.AgentRegistrationChallenge) error {
	tx.challenge = challenge
	return nil
}
func (tx *openRegistrationMemoryTx) GetChallengeForUpdate(_ context.Context, id string) (*domain.AgentRegistrationChallenge, error) {
	if tx.challenge == nil || tx.challenge.ID != id {
		return nil, domain.ErrNotFound
	}
	copy := *tx.challenge
	copy.PublicKey = append([]byte(nil), tx.challenge.PublicKey...)
	copy.Nonce = append([]byte(nil), tx.challenge.Nonce...)
	return &copy, nil
}
func (tx *openRegistrationMemoryTx) ConsumeChallenge(_ context.Context, challenge *domain.AgentRegistrationChallenge) error {
	tx.challenge = challenge
	return nil
}
func (tx *openRegistrationMemoryTx) PublicKeyRegistered(context.Context, string) (bool, error) {
	return tx.key != nil, nil
}
func (tx *openRegistrationMemoryTx) GetGlobalAgentSessionByThumbprint(context.Context, string, string) (*application.GlobalAgentSession, error) {
	return tx.GetGlobalAgentSession(context.Background(), tx.identity.ID, tx.version.ID, tx.membership.OrganizationID)
}
func (tx *openRegistrationMemoryTx) InsertIdentity(_ context.Context, identity *domain.AgentIdentity) error {
	tx.identity = identity
	return nil
}
func (tx *openRegistrationMemoryTx) InsertVersion(_ context.Context, version *domain.AgentVersion) error {
	tx.version = version
	return nil
}
func (tx *openRegistrationMemoryTx) SetIdentityCurrentVersion(_ context.Context, _, versionID string) error {
	if tx.identity == nil || tx.version == nil || tx.version.ID != versionID {
		return domain.ErrNotFound
	}
	tx.identity.CurrentVersionID = versionID
	return nil
}
func (tx *openRegistrationMemoryTx) InsertIdentityKey(_ context.Context, key *domain.AgentIdentityKey) error {
	tx.key = key
	return nil
}
func (tx *openRegistrationMemoryTx) InsertMembership(_ context.Context, membership *domain.OrganizationMembership) error {
	tx.membership = membership
	return nil
}
func (tx *openRegistrationMemoryTx) AppendIdentityEvent(_ context.Context, event domain.AgentIdentityEvent) error {
	tx.events = append(tx.events, event)
	return nil
}
func (tx *openRegistrationMemoryTx) GetGlobalAgentSession(context.Context, string, string, string) (*application.GlobalAgentSession, error) {
	if tx.identity == nil || tx.version == nil || tx.membership == nil {
		return nil, domain.ErrTokenRevoked
	}
	return &application.GlobalAgentSession{
		Identity: tx.identity, Version: tx.version, Scopes: append([]string(nil), tx.membership.Scopes...),
	}, nil
}
func (tx *openRegistrationMemoryTx) TouchGlobalAgent(_ context.Context, agentID, versionID, organizationID string, now time.Time) (*application.GlobalAgentSession, error) {
	session, err := tx.GetGlobalAgentSession(context.Background(), agentID, versionID, organizationID)
	if err != nil {
		return nil, err
	}
	session.Identity.LastSeenAt = &now
	return session, nil
}

var _ application.OpenRegistrationStore = (*openRegistrationMemoryStore)(nil)
var _ application.OpenRegistrationTx = (*openRegistrationMemoryTx)(nil)
var _ application.GlobalTokenIssuer = (*openRegistrationIssuer)(nil)
