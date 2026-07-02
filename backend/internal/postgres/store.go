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
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wrapped := &Tx{tx: tx, acquiredIdempotency: make(map[application.IdempotencyKey]string)}
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

type Tx struct {
	tx pgx.Tx

	nowOnce sync.Once
	now     time.Time
	nowErr  error

	acquiredIdempotency map[application.IdempotencyKey]string
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
