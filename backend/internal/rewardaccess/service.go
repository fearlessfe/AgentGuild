// Package rewardaccess 是奖励应用服务与 transport 之间的授权粘接层。
//
// rewardapp.Service 的读取方法按 resource_tenant_id 取数，而 REST/MCP 只拿到
// 全局标识（公共任务 ID、锁 ID、争议 ID）。这里负责把标识翻译成租户，再决定
// 调用方是否有权访问该资源，从而让两个 transport 共享同一套授权判断，
// 不必各写一遍。
package rewardaccess

import (
	"context"

	coreapp "agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
	rewardpostgres "agentguild.dev/agentguild/backend/internal/reward/postgres"
)

// Resolver 把全局标识翻译成资源真正所属的租户。
type Resolver interface {
	PublicTask(ctx context.Context, publicTaskID string) (rewardpostgres.PublicTaskRef, error)
	Lock(ctx context.Context, lockID string) (rewardpostgres.LockRef, error)
	Dispute(ctx context.Context, disputeID string) (rewardpostgres.LockRef, error)
}

// Rewards 是本层依赖的奖励应用服务子集。
type Rewards interface {
	TopUpEscrow(context.Context, auth.Principal, rewardapp.TopUpEscrow) (rewardapp.Envelope[rewardapp.EscrowView], error)
	GetEscrow(context.Context, auth.Principal, string) (rewardapp.Envelope[rewardapp.EscrowView], error)
	CreateRewardPolicy(context.Context, auth.Principal, rewardapp.CreateRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error)
	FundRewardPolicy(context.Context, auth.Principal, rewardapp.FundRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error)
	GetTaskReward(ctx context.Context, resourceTenantID, taskID string) (rewardapp.Envelope[rewardapp.PolicyView], error)
	GetExecutionReward(ctx context.Context, resourceTenantID, executionID string) (rewardapp.Envelope[rewardapp.LockView], error)
	GetDecision(ctx context.Context, decisionHash string) (rewardapp.Envelope[rewardapp.DecisionView], error)
	OpenDispute(context.Context, auth.Principal, rewardapp.OpenDispute) (rewardapp.Envelope[rewardapp.DisputeView], error)
	ResolveDispute(context.Context, auth.Principal, rewardapp.ResolveDispute) (rewardapp.Envelope[rewardapp.DisputeView], error)
	ChallengePayoutDestination(context.Context, auth.Principal, rewardapp.ChallengePayoutDestination) (rewardapp.Envelope[rewardapp.ChallengeView], error)
	VerifyPayoutDestination(context.Context, auth.Principal, rewardapp.VerifyPayoutDestination) (rewardapp.Envelope[rewardapp.DestinationView], error)
	ListPayoutDestinations(context.Context, auth.Principal) (rewardapp.Envelope[rewardapp.DestinationPage], error)
}

// Service 组合奖励应用服务、标识翻译与跨租户 grant 授权。
type Service struct {
	rewards       Rewards
	resolver      Resolver
	participation coreapp.ParticipationAuthorizer
}

func NewService(rewards Rewards, resolver Resolver, participation coreapp.ParticipationAuthorizer) (*Service, error) {
	if rewards == nil || resolver == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &Service{rewards: rewards, resolver: resolver, participation: participation}, nil
}

// ---------------------------------------------------------------------------
// 匿名可读
// ---------------------------------------------------------------------------

// PublicTaskReward 返回公共任务的奖励契约。它是匿名端点：
// PolicyView 里没有 resource_tenant_id、sponsor 账户或余额，
// 因此翻译出来的租户不会随响应泄露出去。
func (s *Service) PublicTaskReward(ctx context.Context, publicTaskID string) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	ref, err := s.resolver.PublicTask(ctx, publicTaskID)
	if err != nil {
		return rewardapp.Envelope[rewardapp.PolicyView]{}, err
	}
	return s.rewards.GetTaskReward(ctx, ref.ResourceTenantID, ref.TaskID)
}

// Decision 按 decision_hash 返回决策，任何人都可以据此独立验证签名。
func (s *Service) Decision(ctx context.Context, decisionHash string) (rewardapp.Envelope[rewardapp.DecisionView], error) {
	return s.rewards.GetDecision(ctx, decisionHash)
}

// ---------------------------------------------------------------------------
// 需要授权
// ---------------------------------------------------------------------------

// ExecutionReward 返回某次执行的锁定摘要。授权形状与验收证据一致：
// 租户内主体按自身租户读取，全局 Agent 必须持有该 Execution 的任务级 grant。
func (s *Service) ExecutionReward(ctx context.Context, principal auth.Principal, executionID string) (rewardapp.Envelope[rewardapp.LockView], error) {
	var envelope rewardapp.Envelope[rewardapp.LockView]
	if executionID == "" {
		return envelope, domain.ErrInvalidArgument
	}
	tenantID, err := s.executionTenant(ctx, principal, executionID)
	if err != nil {
		return envelope, err
	}
	return s.rewards.GetExecutionReward(ctx, tenantID, executionID)
}

// OpenDispute 在挑战期内对某条决策发起争议。
func (s *Service) OpenDispute(ctx context.Context, principal auth.Principal, lockID, reason string) (rewardapp.Envelope[rewardapp.DisputeView], error) {
	var envelope rewardapp.Envelope[rewardapp.DisputeView]
	ref, err := s.resolver.Lock(ctx, lockID)
	if err != nil {
		return envelope, err
	}
	if err := s.authorizeLock(ctx, principal, ref); err != nil {
		return envelope, err
	}
	return s.rewards.OpenDispute(ctx, principal, rewardapp.OpenDispute{
		ResourceTenantID: ref.ResourceTenantID, LockID: ref.LockID, Reason: reason,
	})
}

// ResolveDispute 裁决争议。它是管理动作：只有资源所属租户的管理员可以裁决，
// 否则一个租户的管理员就能动另一个租户的资金。
func (s *Service) ResolveDispute(ctx context.Context, principal auth.Principal, disputeID, resolution string, agentAmountMinor int64) (rewardapp.Envelope[rewardapp.DisputeView], error) {
	var envelope rewardapp.Envelope[rewardapp.DisputeView]
	ref, err := s.resolver.Dispute(ctx, disputeID)
	if err != nil {
		return envelope, err
	}
	if !principal.IsAdmin || principal.TenantID != ref.ResourceTenantID {
		return envelope, domain.ErrForbidden
	}
	return s.rewards.ResolveDispute(ctx, principal, rewardapp.ResolveDispute{
		ResourceTenantID: ref.ResourceTenantID, DisputeID: disputeID,
		Resolution: resolution, AgentAmountMinor: agentAmountMinor,
	})
}

// ---------------------------------------------------------------------------
// 直通：授权已经由应用服务按 principal 完成
// ---------------------------------------------------------------------------

func (s *Service) GetEscrow(ctx context.Context, principal auth.Principal, currency string) (rewardapp.Envelope[rewardapp.EscrowView], error) {
	return s.rewards.GetEscrow(ctx, principal, currency)
}

func (s *Service) TopUpEscrow(ctx context.Context, principal auth.Principal, command rewardapp.TopUpEscrow) (rewardapp.Envelope[rewardapp.EscrowView], error) {
	return s.rewards.TopUpEscrow(ctx, principal, command)
}

func (s *Service) CreateRewardPolicy(ctx context.Context, principal auth.Principal, command rewardapp.CreateRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	return s.rewards.CreateRewardPolicy(ctx, principal, command)
}

func (s *Service) FundRewardPolicy(ctx context.Context, principal auth.Principal, command rewardapp.FundRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error) {
	return s.rewards.FundRewardPolicy(ctx, principal, command)
}

func (s *Service) ChallengePayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.ChallengePayoutDestination) (rewardapp.Envelope[rewardapp.ChallengeView], error) {
	return s.rewards.ChallengePayoutDestination(ctx, principal, command)
}

func (s *Service) VerifyPayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.VerifyPayoutDestination) (rewardapp.Envelope[rewardapp.DestinationView], error) {
	return s.rewards.VerifyPayoutDestination(ctx, principal, command)
}

func (s *Service) ListPayoutDestinations(ctx context.Context, principal auth.Principal) (rewardapp.Envelope[rewardapp.DestinationPage], error) {
	return s.rewards.ListPayoutDestinations(ctx, principal)
}

// ---------------------------------------------------------------------------
// 授权
// ---------------------------------------------------------------------------

// executionTenant 复用与验收证据一致的资源授权形状。
func (s *Service) executionTenant(ctx context.Context, principal auth.Principal, executionID string) (string, error) {
	if !principal.IsGlobalAgent() {
		if principal.TenantID == "" {
			return "", domain.ErrForbidden
		}
		return principal.TenantID, nil
	}
	if s.participation == nil {
		return "", domain.ErrForbidden
	}
	grant, err := s.participation.Authorize(ctx, principal,
		participationdomain.ResourceExecution, executionID, participationdomain.ScopeExecutionRead)
	if err != nil {
		return "", err
	}
	return grant.ResourceTenantID, nil
}

// authorizeLock 校验调用方能否对该锁发起争议：租户内主体必须同租户，
// 全局 Agent 必须持有对应 Execution 的 grant，且 grant 指向同一租户。
func (s *Service) authorizeLock(ctx context.Context, principal auth.Principal, ref rewardpostgres.LockRef) error {
	tenantID, err := s.executionTenant(ctx, principal, ref.ExecutionID)
	if err != nil {
		return err
	}
	if tenantID != ref.ResourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}
