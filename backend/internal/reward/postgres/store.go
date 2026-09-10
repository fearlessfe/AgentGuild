// Package postgres 提供奖励账本的 PostgreSQL 存储实现。
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/application"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
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

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wrapped := &Tx{q: tx}
	if err := fn(wrapped); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Tx 把一个 pgx 事务包装成应用层的仓储集合。奖励模块的所有命令都在
// 单个事务里完成：资金账本与状态机必须原子推进。
type Tx struct {
	q queryer

	nowOnce sync.Once
	now     time.Time
	nowErr  error
}

// NewTx 让 publictask 的 Claim 事务能够复用同一批仓储实现，
// 在已持锁的事务里写入 RewardLock 而不另开事务。
func NewTx(q queryer) *Tx { return &Tx{q: q} }

func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
	tx.nowOnce.Do(func() {
		tx.nowErr = tx.q.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	})
	return tx.now, tx.nowErr
}

func (tx *Tx) Escrow() application.EscrowRepository      { return &escrowRepository{q: tx.q} }
func (tx *Tx) Policies() application.PolicyRepository    { return &policyRepository{q: tx.q} }
func (tx *Tx) Locks() application.LockRepository         { return &lockRepository{q: tx.q} }
func (tx *Tx) Decisions() application.DecisionRepository { return &decisionRepository{q: tx.q} }
func (tx *Tx) Disputes() application.DisputeRepository   { return &disputeRepository{q: tx.q} }
func (tx *Tx) Receipts() application.ReceiptRepository   { return &receiptRepository{q: tx.q} }
func (tx *Tx) Destinations() application.DestinationRepository {
	return &destinationRepository{q: tx.q}
}

var _ application.Tx = (*Tx)(nil)
var _ application.Store = (*Store)(nil)

// writeError 把数据库层的完整性错误翻译成领域错误。
//
// 余额非负的 CHECK 是"资金不足"唯一可信的判定点：应用层的预检查会被并发
// 打穿，只有这条约束不会。
func writeError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.ConstraintName == "sponsor_escrow_accounts_balances_nonnegative":
		return domain.ErrInsufficientEscrow
	case pgErr.Code == "23505":
		return domain.ErrStateConflict
	case pgErr.Code == "23514" || pgErr.Code == "23503":
		return domain.ErrInvalidArgument
	default:
		return err
	}
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// randomID 供不依赖应用服务的组件（如 Claim 参与者）生成标识。
func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
