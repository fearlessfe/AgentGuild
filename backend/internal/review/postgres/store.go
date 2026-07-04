package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/review/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

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

	wrapped := &Tx{tx: tx}
	if err := fn(wrapped); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

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
	return &reviewRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) LineComments() application.LineCommentRepository {
	return &lineCommentRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) Rubrics() application.RubricRepository {
	return &rubricRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) Reviewers() application.ReviewerRepository {
	return &reviewerRepository{q: tx.tx, now: tx.Now}
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
