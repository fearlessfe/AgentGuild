package application

import (
	"context"
	"strconv"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type OpenDispute struct {
	ResourceTenantID string
	LockID           string
	Reason           string
}

// OpenDispute 在挑战期内对某条决策发起争议，锁进入 disputed 并暂停释放。
//
// 幂等：一个锁最多一条争议，重复发起返回既有争议而不是新建一条。
func (s *Service) OpenDispute(ctx context.Context, principal auth.Principal, command OpenDispute) (Envelope[DisputeView], error) {
	if command.ResourceTenantID == "" || command.LockID == "" {
		return Envelope[DisputeView]{}, invalid("lock_id")
	}
	actor := disputeActor(principal)
	if actor == "" {
		return Envelope[DisputeView]{}, domain.ErrForbidden
	}

	var view DisputeView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		if existing, err := tx.Disputes().GetByLock(ctx, command.ResourceTenantID, command.LockID); err == nil {
			view = disputeView(*existing)
			return nil
		} else if !isNotFound(err) {
			return err
		}
		lock, err := tx.Locks().GetByID(ctx, command.ResourceTenantID, command.LockID)
		if err != nil {
			return err
		}
		decision, err := tx.Decisions().GetByLock(ctx, command.ResourceTenantID, command.LockID)
		if err != nil {
			return err
		}
		// 挑战期已过就不能再发起争议：否则挑战期形同虚设。
		if now.After(decision.ChallengeDeadline) {
			return domain.ErrStateConflict
		}
		dispute, err := domain.NewDispute(domain.NewDisputeParams{
			ID: s.newID(), ResourceTenantID: command.ResourceTenantID,
			LockID: lock.ID, DecisionHash: decision.DecisionHash,
			Reason: command.Reason, OpenedBy: actor, OpenedAt: now,
		})
		if err != nil {
			return err
		}
		if err := lock.Apply(domain.LockIntentDispute, now); err != nil {
			return err
		}
		if err := tx.Locks().Save(ctx, lock); err != nil {
			return err
		}
		if err := tx.Disputes().Insert(ctx, dispute); err != nil {
			return err
		}
		if err := tx.Disputes().AppendEvent(ctx, DisputeEvent{
			DisputeID: dispute.ID, EventType: "opened", ActorID: actor,
			Payload:    map[string]string{"reason": command.Reason},
			OccurredAt: now,
		}); err != nil {
			return err
		}
		view = disputeView(*dispute)
		return nil
	})
	if err != nil {
		return Envelope[DisputeView]{}, err
	}
	return Envelope[DisputeView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

type ResolveDispute struct {
	ResourceTenantID string
	DisputeID        string
	Resolution       string
	AgentAmountMinor int64
}

// ResolveDispute 裁决争议并立即结算。
//
// 裁决只重新分配已托管的资金：release/split 走支付路径，refund 全额退回。
// 已裁决的争议重复投递返回既有结果，不会二次动账（doc §10）。
func (s *Service) ResolveDispute(ctx context.Context, principal auth.Principal, command ResolveDispute) (Envelope[DisputeView], error) {
	if command.ResourceTenantID == "" || command.DisputeID == "" {
		return Envelope[DisputeView]{}, invalid("dispute_id")
	}
	if !principal.IsAdmin {
		return Envelope[DisputeView]{}, domain.ErrForbidden
	}
	actor := disputeActor(principal)
	if actor == "" {
		return Envelope[DisputeView]{}, domain.ErrForbidden
	}

	var resolved *domain.Dispute
	var lockID string
	var alreadyResolved bool
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		dispute, err := tx.Disputes().GetByID(ctx, command.ResourceTenantID, command.DisputeID)
		if err != nil {
			return err
		}
		lockID = dispute.LockID
		if dispute.Status == domain.DisputeResolved {
			resolved, alreadyResolved = dispute, true
			return nil
		}
		decision, err := tx.Decisions().GetByHash(ctx, dispute.DecisionHash)
		if err != nil {
			return err
		}
		if err := dispute.Resolve(domain.DisputeResolution(command.Resolution), actor,
			command.AgentAmountMinor, decision.Allocation.NetAmountMinor, now); err != nil {
			return err
		}
		if err := tx.Disputes().Save(ctx, dispute); err != nil {
			return err
		}
		if err := tx.Disputes().AppendEvent(ctx, DisputeEvent{
			DisputeID: dispute.ID, EventType: "resolved", ActorID: actor,
			Payload: map[string]string{
				"resolution":   command.Resolution,
				"agent_amount": strconv.FormatInt(command.AgentAmountMinor, 10),
			},
			OccurredAt: now,
		}); err != nil {
			return err
		}
		resolved = dispute
		return nil
	})
	if err != nil {
		return Envelope[DisputeView]{}, err
	}
	if alreadyResolved {
		return Envelope[DisputeView]{Data: disputeView(*resolved), Meta: Meta{ServerTime: serverTime}}, nil
	}

	intent, err := resolved.LockIntentFor()
	if err != nil {
		return Envelope[DisputeView]{}, err
	}
	settle := SettleReward{ResourceTenantID: command.ResourceTenantID, LockID: lockID}
	if intent == domain.LockIntentRelease {
		amount := command.AgentAmountMinor
		settle.AgentAmountOverride = &amount
		_, err = s.Release(ctx, settle)
	} else {
		_, err = s.Refund(ctx, settle)
	}
	if err != nil {
		return Envelope[DisputeView]{}, err
	}
	return Envelope[DisputeView]{Data: disputeView(*resolved), Meta: Meta{ServerTime: serverTime}}, nil
}

// disputeActor 返回发起/裁决者标识。Agent 用 agent_id，人类用 owner_id。
func disputeActor(principal auth.Principal) string {
	if principal.AgentID != "" {
		return principal.AgentID
	}
	if principal.OwnerID != "" {
		return principal.OwnerID
	}
	return principal.SubjectID
}
