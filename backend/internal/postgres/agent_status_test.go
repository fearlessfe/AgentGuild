package postgres_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	corepostgres "agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestRequireLiveAgentUsesGlobalIdentityAndActualVersionWithoutTenant(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES
			('agent-active','agent-active','Active','active'),
			('agent-revoked','agent-revoked','Revoked','revoked');
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model
		) VALUES
			('version-active','agent-active',1,'active','pi','gpt-5'),
			('version-retired','agent-active',2,'retired','pi','gpt-5'),
			('version-revoked-agent','agent-revoked',1,'active','pi','gpt-5')`)
	require.NoError(t, err)
	store := corepostgres.NewStore(db)

	tests := []struct {
		name      string
		agentID   string
		versionID string
		code      string
	}{
		{"active", "agent-active", "version-active", ""},
		{"wrong owner", "agent-revoked", "version-active", "forbidden"},
		{"retired version", "agent-active", "version-retired", "state_conflict"},
		{"revoked agent", "agent-revoked", "version-revoked-agent", "token_revoked"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			principal := auth.Principal{
				SubjectID: auth.AgentSubject(test.agentID), IdentityScope: auth.IdentityScopeGlobal,
				Type: auth.PrincipalTypeAgent, AgentID: test.agentID, AgentVersionID: test.versionID,
			}
			err := store.WithTx(ctx, func(tx application.Tx) error {
				return tx.RequireLiveAgent(ctx, principal)
			})
			if test.code == "" {
				require.NoError(t, err)
				return
			}
			if test.code == "token_revoked" {
				require.ErrorIs(t, err, identitydomain.ErrTokenRevoked)
				return
			}
			require.Equal(t, test.code, identitydomain.CodeOf(err))
		})
	}
}
