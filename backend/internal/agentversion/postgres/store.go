package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentversion/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements application.Store using a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// WithTx runs fn inside a PostgreSQL transaction.
func (s *Store) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wrapped := &Tx{tx: tx}
	if err := fn(wrapped); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Tx wraps a pgx transaction and exposes the raw query surface.
type Tx struct {
	tx pgx.Tx

	nowOnce sync.Once
	now     time.Time
	nowErr  error
}

// Now returns the transaction-time clock_timestamp, cached for the lifetime of
// the transaction.
func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

func (tx *Tx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return tx.tx.Exec(ctx, sql, args...)
}

func (tx *Tx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return tx.tx.Query(ctx, sql, args...)
}

func (tx *Tx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return tx.tx.QueryRow(ctx, sql, args...)
}

// ExperienceCandidateProvider implements application.ExperienceCandidateProvider
// using the agentversion postgres store. It reads approved experience candidates
// from the experience_candidates table and supports transactional reads so that
// CreateDraft can validate candidate statuses inside its own transaction.
type ExperienceCandidateProvider struct {
	q queryer
}

// NewExperienceCandidateProvider creates a provider backed by pool.
func NewExperienceCandidateProvider(pool *pgxpool.Pool) *ExperienceCandidateProvider {
	return &ExperienceCandidateProvider{q: pool}
}

// ListApprovedByAgent returns the approved candidates for the agent.
func (p *ExperienceCandidateProvider) ListApprovedByAgent(ctx context.Context, tenantID, agentID string) ([]application.ExperienceCandidateRef, error) {
	return p.listApproved(ctx, p.q, tenantID, agentID)
}

// ListApprovedByAgentTx returns the approved candidates for the agent using the
// given transaction.
func (p *ExperienceCandidateProvider) ListApprovedByAgentTx(ctx context.Context, tx application.Tx, tenantID, agentID string) ([]application.ExperienceCandidateRef, error) {
	return p.listApproved(ctx, tx, tenantID, agentID)
}

func (p *ExperienceCandidateProvider) listApproved(ctx context.Context, q queryer, tenantID, agentID string) ([]application.ExperienceCandidateRef, error) {
	rows, err := q.Query(ctx, `
		SELECT id, evidence_ref
		FROM experience_candidates
		WHERE tenant_id=$1 AND agent_id=$2 AND status='approved'
		ORDER BY created_at DESC`,
		tenantID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []application.ExperienceCandidateRef
	for rows.Next() {
		var ref application.ExperienceCandidateRef
		if err := rows.Scan(&ref.ID, &ref.EvidenceRef); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
var _ application.ExperienceCandidateProvider = (*ExperienceCandidateProvider)(nil)
