package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool                    *pgxpool.Pool
	idempotencyPendingLease time.Duration
}

func NewStore(pool *pgxpool.Pool) *Store {
	return NewStoreWithIdempotencyPendingLease(pool, idempotencyPendingLease)
}

// NewStoreWithIdempotencyPendingLease permits short, deterministic lease
// windows in high-fidelity tests. Production callers should use NewStore.
func NewStoreWithIdempotencyPendingLease(pool *pgxpool.Pool, lease time.Duration) *Store {
	if lease <= 0 {
		lease = idempotencyPendingLease
	}
	return &Store{pool: pool, idempotencyPendingLease: lease}
}

func (s *Store) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wrapped := s.wrapTx(tx)
	if err := fn(wrapped); err != nil {
		return err
	}
	if len(wrapped.acquiredIdempotency) != 0 {
		return &domain.Error{
			Code:    "idempotency_incomplete",
			Message: "acquired idempotency record must be completed before commit",
		}
	}
	return tx.Commit(ctx)
}

// AcquireIdempotency acquires a durable request record in its own short
// transaction. Callers can therefore perform external work after this method
// returns without holding a database transaction open.
func (s *Store) AcquireIdempotency(ctx context.Context, key application.IdempotencyKey, hash [32]byte, expiresAt time.Time) (*application.IdempotencyRecord, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wrapper := s.wrapTx(tx)
	record, err := wrapper.AcquireIdempotency(ctx, key, hash, expiresAt)
	if err != nil {
		return nil, err
	}
	// This transaction intentionally commits the pending lease. Completion is
	// performed by CompleteIdempotency after the external mutation finishes.
	delete(wrapper.acquiredIdempotency, key)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return record, nil
}

// AcquireMutationIdempotency derives retention from the database clock in the
// same short transaction that creates or reuses the REST mutation record. The
// core application transaction API continues to accept an absolute expiry.
func (s *Store) AcquireMutationIdempotency(ctx context.Context, key application.IdempotencyKey, hash [32]byte, ttl time.Duration) (*application.IdempotencyRecord, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wrapper := s.wrapTx(tx)
	now, err := wrapper.Now(ctx)
	if err != nil {
		return nil, err
	}
	record, err := wrapper.AcquireIdempotency(ctx, key, hash, now.Add(ttl))
	if err != nil {
		return nil, err
	}
	delete(wrapper.acquiredIdempotency, key)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return record, nil
}

// RenewIdempotency extends a pending mutation lease only for its current
// owner. Completed records and stale owners are rejected atomically.
func (s *Store) RenewIdempotency(ctx context.Context, key application.IdempotencyKey, owner string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wrapper := s.wrapTx(tx)
	if err := wrapper.RenewIdempotency(ctx, key, owner); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CompleteIdempotency stores the exact HTTP response in a separate short
// transaction after the mutation has finished.
func (s *Store) CompleteIdempotency(ctx context.Context, key application.IdempotencyKey, owner string, status int, body []byte) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wrapper := s.wrapTx(tx)
	if err := wrapper.CompleteIdempotency(ctx, key, owner, status, body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Tx struct {
	tx                      pgx.Tx
	idempotencyPendingLease time.Duration

	nowOnce sync.Once
	now     time.Time
	nowErr  error

	acquiredIdempotency map[application.IdempotencyKey]string
}

func (s *Store) wrapTx(tx pgx.Tx) *Tx {
	return &Tx{
		tx:                      tx,
		idempotencyPendingLease: s.idempotencyPendingLease,
		acquiredIdempotency:     make(map[application.IdempotencyKey]string),
	}
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
