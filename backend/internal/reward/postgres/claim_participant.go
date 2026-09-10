package postgres

import (
	"context"
	"errors"
	"time"

	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/jackc/pgx/v5"
)

// ClaimParticipant 在 publictask 的 Claim 事务里锁定奖励。
//
// 它刻意不开自己的事务：奖励锁定与任务状态、Execution、grant 必须一起
// 提交或一起回滚。余额不足时返回错误，整个 Claim 失败、任务保持 open
// ——不允许无资金背书的 claim。
type ClaimParticipant struct {
	newID   func() string
	lockTTL time.Duration
}

type ClaimParticipantOptions struct {
	NewID func() string
	// LockTTL 是锁在无人决策时自动过期的兜底时长；实际过期时刻取它与
	// 任务 deadline 中较早的一个。
	LockTTL time.Duration
}

func NewClaimParticipant(options ClaimParticipantOptions) *ClaimParticipant {
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.LockTTL <= 0 {
		options.LockTTL = 30 * 24 * time.Hour
	}
	return &ClaimParticipant{newID: options.NewID, lockTTL: options.LockTTL}
}

// OnClaim 在已持锁的事务里校验 escrow 余额并写入 RewardLock。
//
// 任务没有 funded 的 policy 时是 no-op：无奖励任务照常可以领取。
func (p *ClaimParticipant) OnClaim(ctx context.Context, tx pgx.Tx, participation publictaskpostgres.ClaimParticipation) error {
	rewardTx := NewTx(tx)
	policy, err := rewardTx.Policies().GetFundedByTaskForUpdate(ctx,
		participation.ResourceTenantID, participation.TaskID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if !participation.Now.Before(policy.ExpiresAt) {
		// 契约已过期：不锁定资金，但也不阻塞任务领取。
		return nil
	}

	// 同一 policy 上尚未交割的锁定额必须与本次锁定一起受出资额约束，
	// 否则一条 funded policy 会被多次 claim 反复超额承诺。
	committed, err := rewardTx.Policies().LockedAmount(ctx,
		participation.ResourceTenantID, policy.ID)
	if err != nil {
		return err
	}
	amount := policy.FundedAmountMinor - committed
	if amount <= 0 {
		return domain.ErrInsufficientEscrow
	}
	if amount > policy.GrossAmountMinor {
		amount = policy.GrossAmountMinor
	}

	expiresAt := participation.Now.Add(p.lockTTL)
	if policy.ExpiresAt.Before(expiresAt) {
		expiresAt = policy.ExpiresAt
	}
	if !participation.Deadline.IsZero() && participation.Deadline.Before(expiresAt) {
		expiresAt = participation.Deadline
	}
	if !expiresAt.After(participation.Now) {
		return domain.ErrStateConflict
	}

	lock, err := domain.NewLock(domain.NewLockParams{
		ID: p.newID(), ResourceTenantID: participation.ResourceTenantID,
		PolicyID: policy.ID, TaskID: participation.TaskID,
		ExecutionID: participation.ExecutionID, AgentID: participation.AgentID,
		AgentVersionID: participation.AgentVersionID,
		// 快照连同 policy_hash 一起冻结：此后 Issue 如何更新都不会改变
		// 本次执行的金额、criterion 权重与挑战期（doc §10）。
		Snapshot: policy.Snapshot(), LockedAmountMinor: amount,
		LockedAt: participation.Now, ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}
	if err := rewardTx.Locks().Insert(ctx, lock); err != nil {
		return err
	}

	// 余额是否充足由 sponsor_escrow_accounts 的非负 CHECK 判定：应用层的
	// 预检查会被并发打穿，只有这条约束不会。
	available, locked, err := domain.Deltas(domain.EntryLock, amount)
	if err != nil {
		return err
	}
	_, err = rewardTx.Escrow().ApplyEntry(ctx, domain.EscrowEntry{
		TenantID: participation.ResourceTenantID, Currency: policy.Currency,
		EntryType: domain.EntryLock, AmountMinor: amount,
		AvailableDelta: available, LockedDelta: locked,
		ReferenceKind: "lock", ReferenceID: lock.ID,
		IdempotencyKey: "lock:" + lock.ID, CreatedAt: participation.Now,
	})
	return err
}

var _ publictaskpostgres.ClaimParticipant = (*ClaimParticipant)(nil)
