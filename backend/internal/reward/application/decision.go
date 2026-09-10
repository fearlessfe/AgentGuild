package application

import (
	"context"
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"agentguild.dev/agentguild/backend/internal/settlement"
)

type DecideReward struct {
	ResourceTenantID string
	ExecutionID      string
	// QualityMultiplierBps 必须落在 Claim 时锁定的区间内，超出直接报错。
	QualityMultiplierBps int
}

// Decide 为一次执行生成 RewardDecision，并把锁推进到 releasable。
//
// 幂等：已经存在决策时直接返回它，不会产生第二条分配结果。
func (s *Service) Decide(ctx context.Context, command DecideReward) (Envelope[DecisionView], error) {
	if command.ResourceTenantID == "" || command.ExecutionID == "" {
		return Envelope[DecisionView]{}, invalid("execution_id")
	}
	if s.evidence == nil {
		return Envelope[DecisionView]{}, invalid("evidence_source")
	}

	var view DecisionView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		lock, err := tx.Locks().GetByExecution(ctx, command.ResourceTenantID, command.ExecutionID)
		if err != nil {
			return err
		}
		if existing, err := tx.Decisions().GetByLock(ctx, command.ResourceTenantID, lock.ID); err == nil {
			view = decisionView(*existing)
			return nil
		} else if !isNotFound(err) {
			return err
		}
		if lock.Status != domain.LockLocked {
			return domain.ErrStateConflict
		}

		evidence, err := s.evidence.DecisionEvidence(ctx, command.ResourceTenantID, command.ExecutionID)
		if err != nil {
			return err
		}
		coverage := evidence.Coverage()
		outcomes := buildOutcomes(lock.PolicySnapshot, evidence.Spec, coverage)
		quality := command.QualityMultiplierBps
		if quality == 0 {
			quality = domain.BasisPointsScale
		}
		allocation, err := domain.Allocate(domain.AllocateParams{
			Snapshot: lock.PolicySnapshot, GrossAmountMinor: lock.LockedAmountMinor,
			Outcomes: outcomes, AllRequiredPassed: coverage.AllRequiredPassed(),
			QualityMultiplierBps: quality,
		})
		if err != nil {
			return err
		}

		recipientRef := domain.UnassignedRecipientRef
		if allocation.AgentAmountMinor > 0 {
			destination, err := tx.Destinations().GetActive(ctx, lock.AgentID)
			if err != nil {
				return err
			}
			recipientRef = destination.RecipientRef
		}

		decision, err := domain.NewDecision(domain.NewDecisionParams{
			ResourceTenantID: command.ResourceTenantID, LockID: lock.ID,
			PolicyHash: lock.PolicyHash, TaskSpecHash: evidence.TaskSpecHash,
			ContributionHash: evidence.ContributionHash, AlgorithmVersion: s.algorithmVersion,
			Currency: lock.Currency, Allocation: allocation, QualityMultiplierBps: quality,
			CriterionResults: outcomes, RequiredCriteriaPassed: coverage.AllRequiredPassed(),
			RecipientRef: recipientRef, ChallengeDeadline: lock.ChallengeDeadline(now),
			DecidedAt: now, Signer: s.signer,
		})
		if err != nil {
			return err
		}
		if err := tx.Decisions().Insert(ctx, decision); err != nil {
			return err
		}
		if err := lock.Apply(domain.LockIntentMarkReleasable, now); err != nil {
			return err
		}
		if err := tx.Locks().Save(ctx, lock); err != nil {
			return err
		}
		view = decisionView(*decision)
		return nil
	})
	if err != nil {
		return Envelope[DecisionView]{}, err
	}
	return Envelope[DecisionView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// buildOutcomes 把 policy 快照里的权重、任务规格里的必需性与 criterion 的
// 最新验证结果对齐成一份逐条结果。
//
// 快照里没有权重的 criterion 权重为 0；规格里存在但无任何结果的 criterion
// Verified=false —— 未验证绝不等于通过。
func buildOutcomes(snapshot domain.PolicySnapshot, spec []contributiondomain.SpecCriterion, coverage contributiondomain.CriterionCoverage) []domain.CriterionOutcome {
	weights := make(map[string]int, len(snapshot.CriterionWeightsBps))
	for _, weight := range snapshot.CriterionWeightsBps {
		weights[weight.CriterionID] = weight.WeightBps
	}
	outcomes := make([]domain.CriterionOutcome, 0, len(spec))
	for _, criterion := range spec {
		outcome := domain.CriterionOutcome{
			CriterionID: criterion.ID, Required: criterion.Critical,
			WeightBps: weights[criterion.ID],
		}
		if result, found := coverage.ResultsByID[criterion.ID]; found {
			outcome.Verified = true
			outcome.Passed = result.Passed
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// GetDecision 按 decision_hash 返回决策，匿名可读、可独立验证。
func (s *Service) GetDecision(ctx context.Context, decisionHash string) (Envelope[DecisionView], error) {
	if decisionHash == "" {
		return Envelope[DecisionView]{}, invalid("decision_hash")
	}
	var view DecisionView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		decision, err := tx.Decisions().GetByHash(ctx, decisionHash)
		if err != nil {
			return err
		}
		view = decisionView(*decision)
		return nil
	})
	if err != nil {
		return Envelope[DecisionView]{}, err
	}
	return Envelope[DecisionView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

type SettleReward struct {
	ResourceTenantID string
	LockID           string
	// AgentAmountOverride 只在争议裁决后使用，正常路径为 nil。
	AgentAmountOverride *int64
}

// Release 在挑战期届满后经 settlement provider 释放奖励。
//
// 提供方调用刻意放在事务之外：provider 以 decision_hash 幂等，即使在两个
// 事务之间崩溃，重跑也只会拿到同一份回执，不会重复付款（doc §10）。
func (s *Service) Release(ctx context.Context, command SettleReward) (Envelope[LockView], error) {
	if command.ResourceTenantID == "" || command.LockID == "" {
		return Envelope[LockView]{}, invalid("lock_id")
	}
	lock, decision, err := s.loadSettlement(ctx, command)
	if err != nil {
		return Envelope[LockView]{}, err
	}
	if lock.IsTerminal() {
		// 已经是终态：重复释放是 no-op，直接回放当前状态。
		return s.lockEnvelope(ctx, command.ResourceTenantID, command.LockID)
	}

	agentAmount := decision.Allocation.AgentAmountMinor
	if command.AgentAmountOverride != nil {
		agentAmount = *command.AgentAmountOverride
	}
	// 离开托管的部分：Agent、maintainer、reviewer 池与平台费。
	paidOut := agentAmount + decision.Allocation.MaintainerAmountMinor +
		decision.Allocation.ReviewerPoolAmountMinor + decision.Allocation.PlatformFeeMinor
	returned := lock.LockedAmountMinor - paidOut
	if returned < 0 {
		return Envelope[LockView]{}, domain.ErrStateConflict
	}

	receipt, err := s.provider.Pay(ctx, settlement.Request{
		DecisionHash: decision.DecisionHash, Kind: settlement.KindPayment,
		Currency: string(decision.Currency), AmountMinor: agentAmount,
		RecipientRef: decision.RecipientRef,
	})
	if err != nil {
		return Envelope[LockView]{}, err
	}
	if err := s.recordSettlement(ctx, *lock, *decision, receipt, domain.LockIntentRelease, paidOut, returned); err != nil {
		return Envelope[LockView]{}, err
	}
	return s.lockEnvelope(ctx, command.ResourceTenantID, command.LockID)
}

// Refund 把整笔锁定退回 sponsor 的可用余额。
func (s *Service) Refund(ctx context.Context, command SettleReward) (Envelope[LockView], error) {
	if command.ResourceTenantID == "" || command.LockID == "" {
		return Envelope[LockView]{}, invalid("lock_id")
	}
	lock, decision, err := s.loadSettlement(ctx, command)
	if err != nil {
		return Envelope[LockView]{}, err
	}
	if lock.IsTerminal() {
		return s.lockEnvelope(ctx, command.ResourceTenantID, command.LockID)
	}
	receipt, err := s.provider.Refund(ctx, settlement.Request{
		DecisionHash: decision.DecisionHash, Kind: settlement.KindRefund,
		Currency: string(decision.Currency), AmountMinor: lock.LockedAmountMinor,
		RecipientRef: decision.RecipientRef,
	})
	if err != nil {
		return Envelope[LockView]{}, err
	}
	if err := s.recordSettlement(ctx, *lock, *decision, receipt, domain.LockIntentRefund, 0, lock.LockedAmountMinor); err != nil {
		return Envelope[LockView]{}, err
	}
	return s.lockEnvelope(ctx, command.ResourceTenantID, command.LockID)
}

// ExpireLock 让超期未决策的锁作废，资金全额回到 sponsor 可用余额。
// 它不调用 provider：从未有任何资金离开托管。
func (s *Service) ExpireLock(ctx context.Context, resourceTenantID, lockID string) error {
	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		lock, err := tx.Locks().GetByID(ctx, resourceTenantID, lockID)
		if err != nil {
			return err
		}
		if lock.IsTerminal() {
			return nil
		}
		if err := lock.Apply(domain.LockIntentExpire, now); err != nil {
			return err
		}
		if err := tx.Locks().Save(ctx, lock); err != nil {
			return err
		}
		return applyEscrowEntry(ctx, tx, *lock, domain.EntryRefund,
			lock.LockedAmountMinor, "lock", lock.ID, "expire:"+lock.ID, now)
	})
}

// loadSettlement 读取结算所需的锁与决策，不做任何修改。
func (s *Service) loadSettlement(ctx context.Context, command SettleReward) (*domain.Lock, *domain.Decision, error) {
	var lock *domain.Lock
	var decision *domain.Decision
	err := s.store.WithTx(ctx, func(tx Tx) error {
		found, err := tx.Locks().GetByID(ctx, command.ResourceTenantID, command.LockID)
		if err != nil {
			return err
		}
		lock = found
		if found.IsTerminal() {
			decision = &domain.Decision{}
			return nil
		}
		decided, err := tx.Decisions().GetByLock(ctx, command.ResourceTenantID, command.LockID)
		if err != nil {
			return err
		}
		decision = decided
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return lock, decision, nil
}

// recordSettlement 在一个事务里落回执、推进锁状态并记账。
//
// 回执与分录都以幂等键写入：provider 的重复回调不会二次改变余额。
func (s *Service) recordSettlement(ctx context.Context, lock domain.Lock, decision domain.Decision, receipt settlement.Receipt, intent domain.LockIntent, paidOut, returned int64) error {
	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		stored, err := domain.NewReceipt(domain.NewReceiptParams{
			ResourceTenantID: lock.ResourceTenantID, DecisionHash: decision.DecisionHash,
			Provider: receipt.Provider, Reference: receipt.Reference,
			Kind: domain.ReceiptKind(receipt.Kind), State: domain.ReceiptState(receipt.State),
			Currency: domain.Currency(receipt.Currency), AmountMinor: receipt.AmountMinor,
			RecipientRef: receipt.RecipientRef, FailureReason: receipt.FailureReason,
			OccurredAt: receipt.OccurredAt,
		})
		if err != nil {
			return err
		}
		if _, err := tx.Receipts().Append(ctx, stored); err != nil {
			return err
		}
		if receipt.State != settlement.StateSettled {
			// 失败或待定的回执只入账本，不推进状态：worker 会重试。
			return nil
		}

		current, err := tx.Locks().GetByID(ctx, lock.ResourceTenantID, lock.ID)
		if err != nil {
			return err
		}
		if current.IsTerminal() {
			return nil
		}
		if err := current.Apply(intent, now); err != nil {
			return err
		}
		if err := tx.Locks().Save(ctx, current); err != nil {
			return err
		}
		if paidOut > 0 {
			if err := applyEscrowEntry(ctx, tx, *current, domain.EntryRelease, paidOut,
				"decision", decision.DecisionHash, "release:"+decision.DecisionHash, now); err != nil {
				return err
			}
		}
		if returned > 0 {
			if err := applyEscrowEntry(ctx, tx, *current, domain.EntryRefund, returned,
				"decision", decision.DecisionHash, "refund:"+decision.DecisionHash, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func applyEscrowEntry(ctx context.Context, tx Tx, lock domain.Lock, entryType domain.EntryType, amount int64, referenceKind, referenceID, idempotencyKey string, now time.Time) error {
	available, locked, err := domain.Deltas(entryType, amount)
	if err != nil {
		return err
	}
	_, err = tx.Escrow().ApplyEntry(ctx, domain.EscrowEntry{
		TenantID: lock.ResourceTenantID, Currency: lock.Currency, EntryType: entryType,
		AmountMinor: amount, AvailableDelta: available, LockedDelta: locked,
		ReferenceKind: referenceKind, ReferenceID: referenceID,
		IdempotencyKey: idempotencyKey, CreatedAt: now,
	})
	return err
}

func (s *Service) lockEnvelope(ctx context.Context, resourceTenantID, lockID string) (Envelope[LockView], error) {
	var view LockView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		lock, err := tx.Locks().GetByID(ctx, resourceTenantID, lockID)
		if err != nil {
			return err
		}
		decisionHash := ""
		if decision, err := tx.Decisions().GetByLock(ctx, resourceTenantID, lockID); err == nil {
			decisionHash = decision.DecisionHash
		} else if !isNotFound(err) {
			return err
		}
		view = lockView(*lock, decisionHash)
		return nil
	})
	if err != nil {
		return Envelope[LockView]{}, err
	}
	return Envelope[LockView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// ListReceipts 返回某条决策的全部回执，供对账使用。
func (s *Service) ListReceipts(ctx context.Context, decisionHash string) ([]ReceiptView, error) {
	if decisionHash == "" {
		return nil, invalid("decision_hash")
	}
	var views []ReceiptView
	err := s.store.WithTx(ctx, func(tx Tx) error {
		receipts, err := tx.Receipts().ListByDecision(ctx, decisionHash)
		if err != nil {
			return err
		}
		views = make([]ReceiptView, len(receipts))
		for index := range receipts {
			views[index] = receiptView(receipts[index])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return views, nil
}
