// Package worker 推进奖励账本里不需要人工介入的状态迁移。
package worker

import (
	"context"
	"log/slog"

	"agentguild.dev/agentguild/backend/internal/reward/application"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

// defaultBatch 限制单次扫描的规模，避免一个租户的积压拖垮整轮。
const defaultBatch = 50

// Worker 每轮做四件事：
//
//  1. 给已交付但尚未决策的锁生成 RewardDecision（locked → releasable）；
//  2. 挑战期届满后经 settlement provider 释放（releasable → released）；
//  3. 让超期未决策的锁作废并退款（→ expired）；
//  4. 与 provider 对账，补齐缺失的回执。
//
// 每一步都幂等：中途崩溃后重跑不会重复付款或重复动账（doc §10）。
type Worker struct {
	service *application.Service
	store   application.Store
	batch   int
	logger  *slog.Logger
}

type Options struct {
	Batch  int
	Logger *slog.Logger
}

func NewWorker(service *application.Service, store application.Store, options Options) (*Worker, error) {
	if service == nil || store == nil {
		return nil, domain.ErrInvalidArgument
	}
	if options.Batch <= 0 {
		options.Batch = defaultBatch
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	return &Worker{service: service, store: store, batch: options.Batch, logger: options.Logger}, nil
}

// RunOnce 执行一轮。单条记录失败只记录日志并继续：一笔卡住的结算不应
// 阻塞其余租户的资金流转。
func (w *Worker) RunOnce(ctx context.Context) error {
	if err := w.decidePending(ctx); err != nil {
		return err
	}
	if err := w.releaseMatured(ctx); err != nil {
		return err
	}
	return w.expireStale(ctx)
}

// decidePending 为仍处于 locked 的锁生成决策。
//
// 证据不足（例如公共任务尚无规格摘要）时跳过，等待下一轮：决策一旦生成
// 就不可改写，宁可晚一轮也不能基于残缺证据下结论。
func (w *Worker) decidePending(ctx context.Context) error {
	locks, err := w.list(ctx, func(tx application.Tx) ([]domain.Lock, error) {
		return tx.Locks().ListByStatus(ctx, domain.LockLocked, w.batch)
	})
	if err != nil {
		return err
	}
	for _, lock := range locks {
		if _, err := w.service.Decide(ctx, application.DecideReward{
			ResourceTenantID: lock.ResourceTenantID, ExecutionID: lock.ExecutionID,
		}); err != nil {
			w.logger.Debug("reward decision deferred",
				"lock_id", lock.ID, "error", err)
		}
	}
	return nil
}

// releaseMatured 释放挑战期已过的决策。争议中的锁不在扫描范围内。
func (w *Worker) releaseMatured(ctx context.Context) error {
	var decisions []domain.Decision
	err := w.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		decisions, err = tx.Decisions().ListReleasable(ctx, now, w.batch)
		return err
	})
	if err != nil {
		return err
	}
	for _, decision := range decisions {
		if _, err := w.service.Release(ctx, application.SettleReward{
			ResourceTenantID: decision.ResourceTenantID, LockID: decision.LockID,
		}); err != nil {
			w.logger.Warn("reward release failed",
				"lock_id", decision.LockID, "error", err)
		}
	}
	return nil
}

// expireStale 让超期未决策的锁作废，资金全额回到 sponsor 可用余额。
func (w *Worker) expireStale(ctx context.Context) error {
	var locks []domain.Lock
	err := w.store.WithTx(ctx, func(tx application.Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		locks, err = tx.Locks().ListExpired(ctx, now, w.batch)
		return err
	})
	if err != nil {
		return err
	}
	for _, lock := range locks {
		if err := w.service.ExpireLock(ctx, lock.ResourceTenantID, lock.ID); err != nil {
			w.logger.Warn("reward lock expiry failed", "lock_id", lock.ID, "error", err)
		}
	}
	return nil
}

func (w *Worker) list(ctx context.Context, fn func(application.Tx) ([]domain.Lock, error)) ([]domain.Lock, error) {
	var locks []domain.Lock
	err := w.store.WithTx(ctx, func(tx application.Tx) error {
		found, err := fn(tx)
		if err != nil {
			return err
		}
		locks = found
		return nil
	})
	return locks, err
}
