package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type ChallengePayoutDestination struct {
	Chain   string
	Address string
}

// ChallengePayoutDestination 签发一次性 nonce。
//
// 绑定拆成 challenge + verify 两步：nonce 必须由服务端签发且只能用一次，
// 单个接口做不到防重放。
func (s *Service) ChallengePayoutDestination(ctx context.Context, principal auth.Principal, command ChallengePayoutDestination) (Envelope[ChallengeView], error) {
	agentID, err := agentPrincipal(principal)
	if err != nil {
		return Envelope[ChallengeView]{}, err
	}
	if command.Chain == "" || command.Address == "" {
		return Envelope[ChallengeView]{}, invalid("address")
	}

	var view ChallengeView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		challenge, err := domain.NewChallenge(domain.NewChallengeParams{
			Nonce: s.newID(), AgentID: agentID, Chain: command.Chain,
			Address: command.Address, IssuedAt: now, TTL: s.challengeTTL,
		})
		if err != nil {
			return err
		}
		if err := tx.Destinations().InsertChallenge(ctx, challenge); err != nil {
			return err
		}
		view = ChallengeView{
			Nonce: challenge.Nonce, Chain: challenge.Chain,
			Address: challenge.Address, ExpiresAt: challenge.ExpiresAt,
		}
		return nil
	})
	if err != nil {
		return Envelope[ChallengeView]{}, err
	}
	return Envelope[ChallengeView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

type VerifyPayoutDestination struct {
	Nonce   string
	Chain   string
	Address string
}

// VerifyPayoutDestination 消费 nonce 并把目的地置为 verified。
//
// 钱包轮换：旧的 verified 目的地在此被撤销。历史 decision 里冻结的
// recipient_ref 不受影响，因此历史归属不变，旧目的地也无法二次领取。
func (s *Service) VerifyPayoutDestination(ctx context.Context, principal auth.Principal, command VerifyPayoutDestination) (Envelope[DestinationView], error) {
	agentID, err := agentPrincipal(principal)
	if err != nil {
		return Envelope[DestinationView]{}, err
	}
	if command.Nonce == "" || command.Chain == "" || command.Address == "" {
		return Envelope[DestinationView]{}, invalid("nonce")
	}

	var view DestinationView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		if err := tx.Destinations().ConsumeChallenge(ctx, command.Nonce, agentID,
			command.Chain, command.Address, now); err != nil {
			return err
		}
		if active, err := tx.Destinations().GetActive(ctx, agentID); err == nil {
			if active.RecipientRef == domain.RecipientRef(command.Chain, command.Address) {
				// 重复验证同一个目的地：幂等返回，不做任何状态变更。
				view = destinationView(*active)
				return nil
			}
			if err := active.Revoke(now); err != nil {
				return err
			}
			if err := tx.Destinations().Save(ctx, active); err != nil {
				return err
			}
		} else if !isNotFound(err) {
			return err
		}
		destination, err := domain.NewDestination(domain.NewDestinationParams{
			ID: s.newID(), AgentID: agentID, Chain: command.Chain,
			Address: command.Address, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := destination.Verify(now); err != nil {
			return err
		}
		if err := tx.Destinations().Insert(ctx, destination); err != nil {
			return err
		}
		view = destinationView(*destination)
		return nil
	})
	if err != nil {
		return Envelope[DestinationView]{}, err
	}
	return Envelope[DestinationView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// ListPayoutDestinations 返回 Agent 自己的全部目的地（含已撤销的历史）。
func (s *Service) ListPayoutDestinations(ctx context.Context, principal auth.Principal) (Envelope[DestinationPage], error) {
	agentID, err := agentPrincipal(principal)
	if err != nil {
		return Envelope[DestinationPage]{}, err
	}
	var page DestinationPage
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		destinations, err := tx.Destinations().ListByAgent(ctx, agentID)
		if err != nil {
			return err
		}
		page.Items = make([]DestinationView, len(destinations))
		for index := range destinations {
			page.Items[index] = destinationView(destinations[index])
		}
		return nil
	})
	if err != nil {
		return Envelope[DestinationPage]{}, err
	}
	return Envelope[DestinationPage]{Data: page, Meta: Meta{ServerTime: serverTime}}, nil
}

func agentPrincipal(principal auth.Principal) (string, error) {
	if principal.Type != auth.PrincipalTypeAgent || principal.AgentID == "" {
		return "", domain.ErrForbidden
	}
	return principal.AgentID, nil
}
