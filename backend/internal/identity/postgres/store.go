package postgres

import (
	"context"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
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

func (tx *Tx) Agents() application.AgentRepository {
	return &agentRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) Versions() application.VersionRepository {
	return &versionRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) Credentials() application.CredentialRepository {
	return &credentialRepository{q: tx.tx, now: tx.Now}
}

func (tx *Tx) Audits() application.AuditRepository {
	return &auditRepository{q: tx.tx, now: tx.Now}
}

var _ application.Store = (*Store)(nil)
var _ application.Tx = (*Tx)(nil)
