package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Store provides transactional access to the git credential repositories.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store backed by pool.
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

// Tx implements application.Tx for a PostgreSQL transaction.
type Tx struct {
	tx pgx.Tx

	nowOnce sync.Once
	now     time.Time
	nowErr  error
}

// Now returns the transaction-consistent database time.
func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

// Credentials returns the credential repository for this transaction.
func (tx *Tx) Credentials() application.CredentialRepository {
	return &credentialRepository{q: tx.tx, now: tx.Now}
}

// Submissions returns the submission repository for this transaction.
func (tx *Tx) Submissions() application.SubmissionRepository {
	return &submissionRepository{q: tx.tx}
}

// ValidationJobs returns the validation job repository for this transaction.
func (tx *Tx) ValidationJobs() application.ValidationJobRepository {
	return &validationJobRepository{q: tx.tx, now: tx.Now}
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
