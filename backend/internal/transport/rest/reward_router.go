package rest

import (
	"context"
	"net/http"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/go-chi/chi/v5"
)

// RewardService 是链下奖励账本的 transport 边界。
// *rewardaccess.Service 天然满足此接口：标识翻译与跨租户授权都在那里完成，
// 这里只负责 HTTP 形状。
type RewardService interface {
	PublicTaskReward(ctx context.Context, publicTaskID string) (rewardapp.Envelope[rewardapp.PolicyView], error)
	Decision(ctx context.Context, decisionHash string) (rewardapp.Envelope[rewardapp.DecisionView], error)
	ExecutionReward(ctx context.Context, principal auth.Principal, executionID string) (rewardapp.Envelope[rewardapp.LockView], error)
	GetEscrow(ctx context.Context, principal auth.Principal, currency string) (rewardapp.Envelope[rewardapp.EscrowView], error)
	TopUpEscrow(ctx context.Context, principal auth.Principal, command rewardapp.TopUpEscrow) (rewardapp.Envelope[rewardapp.EscrowView], error)
	CreateRewardPolicy(ctx context.Context, principal auth.Principal, command rewardapp.CreateRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error)
	FundRewardPolicy(ctx context.Context, principal auth.Principal, command rewardapp.FundRewardPolicy) (rewardapp.Envelope[rewardapp.PolicyView], error)
	OpenDispute(ctx context.Context, principal auth.Principal, lockID, reason string) (rewardapp.Envelope[rewardapp.DisputeView], error)
	ResolveDispute(ctx context.Context, principal auth.Principal, disputeID, resolution string, agentAmountMinor int64) (rewardapp.Envelope[rewardapp.DisputeView], error)
	ChallengePayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.ChallengePayoutDestination) (rewardapp.Envelope[rewardapp.ChallengeView], error)
	VerifyPayoutDestination(ctx context.Context, principal auth.Principal, command rewardapp.VerifyPayoutDestination) (rewardapp.Envelope[rewardapp.DestinationView], error)
	ListPayoutDestinations(ctx context.Context, principal auth.Principal) (rewardapp.Envelope[rewardapp.DestinationPage], error)
}

// WithRewardService 挂载链下奖励账本 REST API。
func WithRewardService(svc RewardService) Option {
	return func(s *Server) { s.rewardSvc = svc }
}

// rewardConfigured 在未装配奖励模块时给出稳定的 501，避免路由被静默吞掉。
func (s *Server) rewardConfigured(w http.ResponseWriter) bool {
	if s.rewardSvc == nil {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "reward ledger is not configured")
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// 匿名可读
// ---------------------------------------------------------------------------

// getPublicTaskReward 公开任务的奖励契约。响应里没有 sponsor 租户、
// sponsor 账户或托管余额，因此匿名可读。
func (s *Server) getPublicTaskReward(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	result, err := s.rewardSvc.PublicTaskReward(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// getRewardDecision 让任何第三方用公开 hash 取回决策并独立验证签名。
func (s *Server) getRewardDecision(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	result, err := s.rewardSvc.Decision(r.Context(), chi.URLParam(r, "decision_hash"))
	if err != nil {
		mapDomainError(w, err, auth.Principal{})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Agent 自服务
// ---------------------------------------------------------------------------

func (s *Server) getExecutionReward(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	result, err := s.rewardSvc.ExecutionReward(r.Context(), principal, chi.URLParam(r, "id"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listPayoutDestinations(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	result, err := s.rewardSvc.ListPayoutDestinations(r.Context(), principal)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// challengePayoutDestination 签发一次性 nonce。绑定拆成 challenge + verify
// 两步：nonce 必须由服务端签发且只能用一次，单个接口做不到防重放。
func (s *Server) challengePayoutDestination(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
		Chain     string `json:"chain"`
		Address   string `json:"address"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.rewardSvc.ChallengePayoutDestination(r.Context(), principal,
		rewardapp.ChallengePayoutDestination{Chain: body.Chain, Address: body.Address})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) verifyPayoutDestination(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
		Nonce     string `json:"nonce"`
		Chain     string `json:"chain"`
		Address   string `json:"address"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.rewardSvc.VerifyPayoutDestination(r.Context(), principal,
		rewardapp.VerifyPayoutDestination{Nonce: body.Nonce, Chain: body.Chain, Address: body.Address})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Sponsor 托管与契约
// ---------------------------------------------------------------------------

func (s *Server) getSponsorEscrow(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	result, err := s.rewardSvc.GetEscrow(r.Context(), principal, r.URL.Query().Get("currency"))
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) topUpSponsorEscrow(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID   string `json:"request_id"`
		Currency    string `json:"currency"`
		AmountMinor int64  `json:"amount_minor"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	requestID, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}
	result, err := s.rewardSvc.TopUpEscrow(r.Context(), principal, rewardapp.TopUpEscrow{
		RequestID: requestID, Currency: body.Currency, AmountMinor: body.AmountMinor,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// rewardPolicyBody 是创建奖励契约的请求体。分成比例一律用整数 basis points：
// 浮点数无法产生稳定的 policy_hash，也无法让第三方跨语言复算决策。
type rewardPolicyBody struct {
	RequestID               string                         `json:"request_id"`
	TaskID                  string                         `json:"task_id"`
	Currency                string                         `json:"currency"`
	GrossAmountMinor        int64                          `json:"gross_amount_minor"`
	CriterionWeights        []rewarddomain.CriterionWeight `json:"criterion_weights_bps"`
	MaintainerShareBps      int                            `json:"maintainer_share_bps"`
	ReviewerPoolShareBps    int                            `json:"reviewer_pool_share_bps"`
	PlatformFeeBps          int                            `json:"platform_fee_bps"`
	DisputeReserveBps       int                            `json:"dispute_reserve_bps"`
	QualityMultiplierMinBps int                            `json:"quality_multiplier_min_bps"`
	QualityMultiplierMaxBps int                            `json:"quality_multiplier_max_bps"`
	ChallengePeriodSeconds  int64                          `json:"challenge_period_seconds"`
	ExpiresAt               time.Time                      `json:"expires_at"`
}

func (s *Server) createRewardPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body rewardPolicyBody
	if !decodeBody(w, r, &body) {
		return
	}
	requestID, ok := resolveIdempotencyKey(r, body.RequestID, w)
	if !ok {
		return
	}
	result, err := s.rewardSvc.CreateRewardPolicy(r.Context(), principal, rewardapp.CreateRewardPolicy{
		RequestID: requestID, TaskID: body.TaskID, Currency: body.Currency,
		GrossAmountMinor: body.GrossAmountMinor, CriterionWeights: body.CriterionWeights,
		MaintainerShareBps: body.MaintainerShareBps, ReviewerPoolShareBps: body.ReviewerPoolShareBps,
		PlatformFeeBps: body.PlatformFeeBps, DisputeReserveBps: body.DisputeReserveBps,
		QualityMultiplierMinBps: body.QualityMultiplierMinBps,
		QualityMultiplierMaxBps: body.QualityMultiplierMaxBps,
		ChallengePeriod:         time.Duration(body.ChallengePeriodSeconds) * time.Second,
		ExpiresAt:               body.ExpiresAt,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) fundRewardPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID   string `json:"request_id"`
		AmountMinor int64  `json:"amount_minor"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.rewardSvc.FundRewardPolicy(r.Context(), principal, rewardapp.FundRewardPolicy{
		PolicyID: chi.URLParam(r, "id"), AmountMinor: body.AmountMinor,
	})
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// 争议
// ---------------------------------------------------------------------------

func (s *Server) openRewardDispute(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID string `json:"request_id"`
		Reason    string `json:"reason"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.rewardSvc.OpenDispute(r.Context(), principal, chi.URLParam(r, "id"), body.Reason)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) resolveRewardDispute(w http.ResponseWriter, r *http.Request) {
	if !s.rewardConfigured(w) {
		return
	}
	principal := mustPrincipal(r)
	var body struct {
		RequestID        string `json:"request_id"`
		Resolution       string `json:"resolution"`
		AgentAmountMinor int64  `json:"agent_amount_minor"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	result, err := s.rewardSvc.ResolveDispute(r.Context(), principal,
		chi.URLParam(r, "id"), body.Resolution, body.AgentAmountMinor)
	if err != nil {
		mapDomainError(w, err, principal)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
