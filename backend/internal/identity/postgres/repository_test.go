package postgres_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestAgentTenantIsolationCannotBeBypassed(t *testing.T) {
	db := testdb.StartPostgres(t)
	insertAgent(t, db, "tenant-1", "agent-1")

	repo := postgres.NewAgentRepository(db)
	_, err := repo.GetByID(context.Background(), "tenant-2", "agent-1")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestVersionTenantIsolationCannotBeBypassed(t *testing.T) {
	db := testdb.StartPostgres(t)
	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	versionID := insertVersion(t, db, "tenant-1", agentID, 1)

	repo := postgres.NewVersionRepository(db)
	_, err := repo.GetByID(context.Background(), "tenant-2", versionID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCredentialTenantIsolationCannotBeBypassed(t *testing.T) {
	db := testdb.StartPostgres(t)
	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	credID, _ := insertCredential(t, db, "tenant-1", agentID)

	repo := postgres.NewCredentialRepository(db)
	_, err := repo.GetPending(context.Background(), "tenant-2", agentID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = repo.GetByID(context.Background(), "tenant-2", credID)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConcurrentActivationConsumesOnlyOneToken(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	store := postgres.NewStore(db)
	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	credID, token, err := domain.NewActivationCredential(agentID, "tenant-1", time.Hour)
	require.NoError(t, err)
	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Insert(ctx, credID)
	}))

	var successes int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.WithTx(ctx, func(tx application.Tx) error {
				cred, err := tx.Credentials().GetPending(ctx, "tenant-1", agentID)
				if err != nil {
					return err
				}
				now, err := tx.Now(ctx)
				if err != nil {
					return err
				}
				if err := cred.Consume(token, now); err != nil {
					return err
				}
				return tx.Credentials().Save(ctx, cred)
			})
			if err == nil {
				atomic.AddInt32(&successes, 1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(1), successes)
}

func TestCredentialConsumeRequiresStoredCredentialIdentity(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	credID, _ := insertCredential(t, db, "tenant-1", agentID)
	repo := postgres.NewCredentialRepository(db)

	forged := &domain.ActivationCredential{
		ID:         "cred-forged",
		TenantID:   "tenant-1",
		AgentID:    agentID,
		Hash:       []byte("not-the-stored-hash"),
		Status:     domain.ActivationCredentialConsumed,
		ConsumedAt: ptrTime(time.Now()),
	}
	err := repo.Save(ctx, forged)
	require.ErrorIs(t, err, domain.ErrTokenExpired)

	got, err := repo.GetByID(ctx, "tenant-1", credID)
	require.NoError(t, err)
	require.Equal(t, domain.ActivationCredentialPending, got.Status)
	require.Nil(t, got.ConsumedAt)
}

func TestCredentialSavePersistsPendingCredentialUpdates(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	credID, _ := insertCredential(t, db, "tenant-1", agentID)
	repo := postgres.NewCredentialRepository(db)

	cred, err := repo.GetByID(ctx, "tenant-1", credID)
	require.NoError(t, err)
	cred.Hash = []byte("rotated-token-hash")
	cred.Status = domain.ActivationCredentialPending
	expiresAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	cred.ExpiresAt = &expiresAt
	cred.ConsumedAt = nil

	require.NoError(t, repo.Save(ctx, cred))

	got, err := repo.GetByID(ctx, "tenant-1", credID)
	require.NoError(t, err)
	require.Equal(t, []byte("rotated-token-hash"), got.Hash)
	require.Equal(t, domain.ActivationCredentialPending, got.Status)
	require.NotNil(t, got.ExpiresAt)
	require.Equal(t, expiresAt, got.ExpiresAt.UTC())
	require.Nil(t, got.ConsumedAt)
}

func TestAuditEventsAppendOnly(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	store := postgres.NewStore(db)

	agentID := insertAgent(t, db, "tenant-1", "agent-1")
	event := domain.NewIdentityEvent(
		"tenant-1", agentID, domain.ActorSystem, "system", "register",
		"", domain.AgentPendingActivation, "", map[string]any{"hello": "world"},
		time.Now(),
	)

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		return tx.Audits().Append(ctx, event)
	}))

	events, err := postgres.NewAuditRepository(db).ListByAgent(ctx, "tenant-1", agentID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "register", events[0].Intent)
	require.Equal(t, "world", events[0].Payload["hello"])
}

func TestActivationPersistsAgentVersionAndEvent(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	store := postgres.NewStore(db)

	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", []string{"tasks:read"}, nil)
	require.NoError(t, err)
	cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)
	version, err := domain.NewAgentVersion(
		"version-1", agent.TenantID, agent.ID, 1, "runtime-1", "model-1",
		[]string{"read"}, "fingerprint-1", time.Now(),
	)
	require.NoError(t, err)

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		if err := tx.Agents().Insert(ctx, agent); err != nil {
			return err
		}
		for _, ev := range agent.Events {
			if err := tx.Audits().Append(ctx, ev); err != nil {
				return err
			}
		}
		if err := tx.Versions().Insert(ctx, version); err != nil {
			return err
		}
		if err := tx.Credentials().Insert(ctx, cred); err != nil {
			return err
		}
		return nil
	}))

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		freshAgent, err := tx.Agents().GetByID(ctx, agent.TenantID, agent.ID)
		if err != nil {
			return err
		}
		freshCred, err := tx.Credentials().GetPending(ctx, agent.TenantID, agent.ID)
		if err != nil {
			return err
		}
		if err := freshAgent.Activate(freshCred, version, token, now); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, freshAgent); err != nil {
			return err
		}
		if err := tx.Credentials().Save(ctx, freshCred); err != nil {
			return err
		}
		for _, ev := range freshAgent.Events {
			if err := tx.Audits().Append(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}))

	got, err := postgres.NewAgentRepository(db).GetByID(ctx, agent.TenantID, agent.ID)
	require.NoError(t, err)
	require.Equal(t, domain.AgentActive, got.Status)
	require.Equal(t, version.ID, got.CurrentVersionID)

	events, err := postgres.NewAuditRepository(db).ListByAgent(ctx, agent.TenantID, agent.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "activate", events[0].Intent)
}

func TestAgentIdentityMigrationCanRollbackAndReapply(t *testing.T) {
	db := testdb.StartPostgres(t)

	testdb.ApplyDownMigration(t, db)
	testdb.ApplyUpMigration(t, db)

	insertAgent(t, db, "tenant-1", "agent-1")
}

func TestIdentityPrimaryAndUniqueConstraintsIncludeTenantID(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT c.conname, rel.relname
		FROM pg_constraint c
		JOIN pg_class rel ON rel.oid = c.conrelid
		WHERE rel.relname = ANY($1::text[])
		  AND c.contype IN ('p', 'u')
		  AND NOT EXISTS (
		      SELECT 1
		      FROM unnest(c.conkey) AS key(attnum)
		      JOIN pg_attribute attr
		        ON attr.attrelid = c.conrelid
		       AND attr.attnum = key.attnum
		      WHERE attr.attname = 'tenant_id'
		  )
		ORDER BY rel.relname, c.conname`,
		[]string{"agents", "agent_versions", "activation_credentials", "identity_events"},
	)
	require.NoError(t, err)
	defer rows.Close()

	var violations []string
	for rows.Next() {
		var constraintName, tableName string
		require.NoError(t, rows.Scan(&constraintName, &tableName))
		violations = append(violations, tableName+"."+constraintName)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, violations)
}

func TestAgentLifecyclePersistence(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	store := postgres.NewStore(db)

	agentID := insertActiveAgent(t, db, "tenant-1", "agent-1")

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fresh, err := tx.Agents().GetByID(ctx, "tenant-1", agentID)
		if err != nil {
			return err
		}
		if err := fresh.Suspend("admin-1", now); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, fresh); err != nil {
			return err
		}
		for _, ev := range fresh.Events {
			if err := tx.Audits().Append(ctx, ev); err != nil {
				return err
			}
		}
		return nil
	}))

	got, err := postgres.NewAgentRepository(db).GetByID(ctx, "tenant-1", agentID)
	require.NoError(t, err)
	require.Equal(t, domain.AgentSuspended, got.Status)

	events, err := postgres.NewAuditRepository(db).ListByAgent(ctx, "tenant-1", agentID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "suspend", events[0].Intent)
}

func TestAgentUpdateRejectsStaleStateTransition(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	store := postgres.NewStore(db)

	agentID := insertActiveAgent(t, db, "tenant-1", "agent-1")
	repo := postgres.NewAgentRepository(db)
	stale, err := repo.GetByID(ctx, "tenant-1", agentID)
	require.NoError(t, err)

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fresh, err := tx.Agents().GetByID(ctx, "tenant-1", agentID)
		if err != nil {
			return err
		}
		if err := fresh.Revoke("admin-1", "compromised", now); err != nil {
			return err
		}
		return tx.Agents().Update(ctx, fresh)
	}))

	require.NoError(t, stale.Suspend("admin-2", time.Now()))
	err = repo.Update(ctx, stale)
	require.ErrorIs(t, err, domain.ErrStateConflict)

	got, err := repo.GetByID(ctx, "tenant-1", agentID)
	require.NoError(t, err)
	require.Equal(t, domain.AgentRevoked, got.Status)
}

func insertAgent(t *testing.T, db *pgxpool.Pool, tenantID, agentID string) string {
	t.Helper()
	ctx := context.Background()
	agent, err := domain.NewAgent(agentID, tenantID, "owner-1", "owner@example.com", "team-a", []string{"tasks:read"}, nil)
	require.NoError(t, err)
	require.NoError(t, postgres.NewStore(db).WithTx(ctx, func(tx application.Tx) error {
		return tx.Agents().Insert(ctx, agent)
	}))
	return agentID
}

func insertActiveAgent(t *testing.T, db *pgxpool.Pool, tenantID, agentID string) string {
	t.Helper()
	ctx := context.Background()
	store := postgres.NewStore(db)
	agent, err := domain.NewAgent(agentID, tenantID, "owner-1", "owner@example.com", "team-a", []string{"tasks:read"}, nil)
	require.NoError(t, err)
	cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)
	version, err := domain.NewAgentVersion(
		"version-"+agentID, tenantID, agentID, 1, "runtime-1", "model-1",
		[]string{"read"}, "fingerprint-1", time.Now(),
	)
	require.NoError(t, err)

	require.NoError(t, store.WithTx(ctx, func(tx application.Tx) error {
		if err := tx.Agents().Insert(ctx, agent); err != nil {
			return err
		}
		if err := tx.Versions().Insert(ctx, version); err != nil {
			return err
		}
		if err := tx.Credentials().Insert(ctx, cred); err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		if err := agent.Activate(cred, version, token, now); err != nil {
			return err
		}
		if err := tx.Agents().Update(ctx, agent); err != nil {
			return err
		}
		return tx.Credentials().Save(ctx, cred)
	}))
	return agentID
}

func insertVersion(t *testing.T, db *pgxpool.Pool, tenantID, agentID string, number int) string {
	t.Helper()
	ctx := context.Background()
	version, err := domain.NewAgentVersion(
		"version-"+agentID, tenantID, agentID, number, "runtime-1", "model-1",
		[]string{"read"}, "fingerprint-1", time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, postgres.NewStore(db).WithTx(ctx, func(tx application.Tx) error {
		return tx.Versions().Insert(ctx, version)
	}))
	return version.ID
}

func insertCredential(t *testing.T, db *pgxpool.Pool, tenantID, agentID string) (string, string) {
	t.Helper()
	ctx := context.Background()
	cred, token, err := domain.NewActivationCredential(agentID, tenantID, time.Hour)
	require.NoError(t, err)
	require.NoError(t, postgres.NewStore(db).WithTx(ctx, func(tx application.Tx) error {
		return tx.Credentials().Insert(ctx, cred)
	}))
	return cred.ID, token
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
