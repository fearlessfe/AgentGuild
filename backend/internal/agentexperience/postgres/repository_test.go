package postgres

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestExperienceCandidateRepository_CreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	repo := NewExperienceCandidateRepository(pool)
	store := NewStore(pool)

	c, err := domain.NewExperienceCandidate(
		"c1", "tenant-1", "agent-1", "task-1", "sub-1", "rev-1",
		"sha256:evidence", []string{"code"}, "tenant-1", time.Now(),
	)
	require.NoError(t, err)

	err = store.WithTx(ctx, func(tx application.Tx) error {
		return repo.Create(ctx, tx, c)
	})
	require.NoError(t, err)

	loaded, err := repo.GetByID(ctx, c.TenantID, c.AgentID, c.ID)
	require.NoError(t, err)
	require.Equal(t, c.ID, loaded.ID)
	require.Equal(t, c.ContentHash, loaded.ContentHash)
	require.Equal(t, domain.StatusPendingReview, loaded.Status)
}

func TestExperienceCandidateRepository_ForbiddenAutoRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	repo := NewExperienceCandidateRepository(pool)
	store := NewStore(pool)

	c, err := domain.NewExperienceCandidate(
		"c1", "tenant-1", "agent-1", "task-1", "sub-1", "rev-1",
		"password = secret", []string{"code"}, "tenant-1", time.Now(),
	)
	require.NoError(t, err)
	_ = c.ClassifyAndApply(domain.NewRuleBasedSensitivityPolicy())

	err = store.WithTx(ctx, func(tx application.Tx) error {
		return repo.Create(ctx, tx, c)
	})
	require.NoError(t, err)

	loaded, err := repo.GetByID(ctx, c.TenantID, c.AgentID, c.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusRejected, loaded.Status)
	require.NotEmpty(t, loaded.PolicyReason)
}

func TestExperienceCandidateRepository_ListByAgentAndStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	repo := NewExperienceCandidateRepository(pool)
	store := NewStore(pool)

	now := time.Now()
	c1, _ := domain.NewExperienceCandidate("c1", "t1", "a1", "task", "sub1", "rev", "ev1", nil, "t1", now)
	c2, _ := domain.NewExperienceCandidate("c2", "t1", "a1", "task", "sub2", "rev", "ev2", nil, "t1", now)
	require.NoError(t, c2.Approve("owner", now))

	err := store.WithTx(ctx, func(tx application.Tx) error {
		if err := repo.Create(ctx, tx, c1); err != nil {
			return err
		}
		return repo.Create(ctx, tx, c2)
	})
	require.NoError(t, err)

	all, err := repo.ListByAgent(ctx, "t1", "a1")
	require.NoError(t, err)
	require.Len(t, all, 2)

	pending, err := repo.ListByAgentAndStatus(ctx, "t1", "a1", domain.StatusPendingReview)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, c1.ID, pending[0].ID)

	approved, err := repo.ListApprovedByAgent(ctx, "t1", "a1")
	require.NoError(t, err)
	require.Len(t, approved, 1)
	require.Equal(t, c2.ID, approved[0].ID)
}

func TestExperienceCandidateRepository_UpdateStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	repo := NewExperienceCandidateRepository(pool)
	store := NewStore(pool)

	c, _ := domain.NewExperienceCandidate("c1", "t1", "a1", "task", "sub", "rev", "ev", nil, "t1", time.Now())
	err := store.WithTx(ctx, func(tx application.Tx) error {
		return repo.Create(ctx, tx, c)
	})
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, c.Approve("owner", now))
	err = store.WithTx(ctx, func(tx application.Tx) error {
		return repo.UpdateStatus(ctx, tx, c)
	})
	require.NoError(t, err)

	loaded, err := repo.GetByID(ctx, c.TenantID, c.AgentID, c.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusApproved, loaded.Status)
	require.Equal(t, "owner", loaded.ReviewedBy)
}

func TestExperienceCandidateRepository_TenantIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	pool := testdb.StartPostgres(t)
	repo := NewExperienceCandidateRepository(pool)
	store := NewStore(pool)

	c, _ := domain.NewExperienceCandidate("c1", "t1", "a1", "task", "sub", "rev", "ev", nil, "t1", time.Now())
	err := store.WithTx(ctx, func(tx application.Tx) error {
		return repo.Create(ctx, tx, c)
	})
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, "t2", c.AgentID, c.ID)
	require.ErrorIs(t, err, domain.ErrNotFound)

	list, err := repo.ListByAgent(ctx, "t2", c.AgentID)
	require.NoError(t, err)
	require.Empty(t, list)
}
