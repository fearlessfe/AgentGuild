package postgres_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestGlobalAgentIdentityMigrationBackfillsDeterministicallyAndRollsBack(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	// The shared harness applies every migration. Remove the dependent
	// Contribution schema so this test can exercise migration 000020's own
	// up/down boundary in isolation.
	_, err := db.Exec(ctx, readIdentityMigration(t, "000027_open_agent_registration.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(ctx, readIdentityMigration(t, "000025_public_task_claim_contract.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(ctx, readIdentityMigration(t, "000024_task_participation_grants.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(ctx, readIdentityMigration(t, "000023_public_task_projections.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(ctx, readIdentityMigration(t, "000022_contribution_projections.down.sql"))
	require.NoError(t, err)
	_, err = db.Exec(ctx, readIdentityMigration(t, "000021_agent_contributions.down.sql"))
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO agents (
			id, tenant_id, owner_id, owner_email, team, name, description, status,
			scopes, repo_scope, budget_cents, budget_currency
		) VALUES (
			'agent-1', 'tenant-1', 'owner-1', 'owner@example.com', 'core',
			'Agent One', 'legacy agent', 'active',
			ARRAY['tasks:read'], ARRAY['owner/repo'], 125, 'USD'
		)`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO agent_versions (
			id, tenant_id, agent_id, version_number, runtime, model, capabilities,
			config_fingerprint, status, content_hash, created_by
		) VALUES (
			'version-1', 'tenant-1', 'agent-1', 1, 'pi', 'gpt-5', ARRAY['go'],
			'sha256:config', 'active', 'sha256:content', 'owner-1'
		)`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		UPDATE agents
		SET current_version_id='version-1'
		WHERE tenant_id='tenant-1' AND id='agent-1'`)
	require.NoError(t, err)

	up := readIdentityMigration(t, "000020_global_agent_identity.up.sql")
	_, err = db.Exec(ctx, up)
	require.NoError(t, err)
	_, err = db.Exec(ctx, up)
	require.NoError(t, err, "migration must be idempotent during backfill retries")

	const globalAgentID = "ag:legacy:8:tenant-1:agent-1"
	const globalVersionID = "agv:legacy:8:tenant-1:version-1"
	var handle, displayName, currentVersionID string
	err = db.QueryRow(ctx, `
		SELECT handle, display_name, current_version_id
		FROM agent_identities
		WHERE id=$1`, globalAgentID,
	).Scan(&handle, &displayName, &currentVersionID)
	require.NoError(t, err)
	require.NotEmpty(t, handle)
	require.Equal(t, "Agent One", displayName)
	require.Equal(t, globalVersionID, currentVersionID)

	var organizationID, operatorID string
	var scopes []string
	err = db.QueryRow(ctx, `
		SELECT organization_id, operator_id, scopes
		FROM agent_organization_memberships
		WHERE agent_id=$1`, globalAgentID,
	).Scan(&organizationID, &operatorID, &scopes)
	require.NoError(t, err)
	require.Equal(t, "tenant-1", organizationID)
	require.Equal(t, "owner-1", operatorID)
	require.Equal(t, []string{"tasks:read"}, scopes)

	var mappedAgentID, mappedVersionID string
	err = db.QueryRow(ctx, `
		SELECT agent_id
		FROM legacy_agent_identity_mappings
		WHERE tenant_id='tenant-1' AND legacy_agent_id='agent-1'`,
	).Scan(&mappedAgentID)
	require.NoError(t, err)
	err = db.QueryRow(ctx, `
		SELECT agent_version_id
		FROM legacy_agent_version_mappings
		WHERE tenant_id='tenant-1' AND legacy_version_id='version-1'`,
	).Scan(&mappedVersionID)
	require.NoError(t, err)
	require.Equal(t, globalAgentID, mappedAgentID)
	require.Equal(t, globalVersionID, mappedVersionID)

	var membershipHasRepoScope bool
	err = db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema=current_schema()
			  AND table_name='agent_organization_memberships'
			  AND column_name='repo_scope'
		)`,
	).Scan(&membershipHasRepoScope)
	require.NoError(t, err)
	require.False(t, membershipHasRepoScope)

	down := readIdentityMigration(t, "000020_global_agent_identity.down.sql")
	_, err = db.Exec(ctx, down)
	require.NoError(t, err)

	var legacyCount int
	err = db.QueryRow(ctx, `
		SELECT count(*) FROM agents
		WHERE tenant_id='tenant-1' AND id='agent-1'`,
	).Scan(&legacyCount)
	require.NoError(t, err)
	require.Equal(t, 1, legacyCount, "rollback must preserve the legacy source of truth")
}

func readIdentityMigration(t *testing.T, name string) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	path := filepath.Join(filepath.Dir(filename), "..", "..", "..", "migrations", name)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
