package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	reviewapplication "agentguild.dev/agentguild/backend/internal/review/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements reviewapplication.Store with a dedicated transaction boundary
// for the code-review repositories. It intentionally does not depend on the
// global postgres package to avoid an import cycle.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) WithTx(ctx context.Context, fn func(reviewapplication.Tx) error) error {
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

// Tx implements reviewapplication.Tx. It exposes only the code-review
// repositories plus transaction-time access; the review application service
// itself runs on the global application.Tx interface.
type Tx struct {
	tx pgx.Tx

	nowOnce sync.Once
	now     time.Time
	nowErr  error
}

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

func (tx *Tx) Reviews() application.ReviewRepository {
	return NewReviewRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) LineComments() application.LineCommentRepository {
	return NewLineCommentRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) Rubrics() application.RubricRepository {
	return NewRubricRepositoryFromTx(tx.tx, tx.Now)
}

func (tx *Tx) Reviewers() application.ReviewerRepository {
	return NewReviewerRepositoryFromTx(tx.tx, tx.Now)
}

var _ reviewapplication.Store = (*Store)(nil)
var _ reviewapplication.Tx = (*Tx)(nil)
