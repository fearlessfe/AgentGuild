package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	coredomain "agentguild.dev/agentguild/backend/internal/domain"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// Meta / Envelope 与 internal/application 的信封形状保持一致。这里重新定义
// 而不是引用，是因为核心应用层已经依赖本包（v1 的 ReputationQueryService），
// 反向引用会形成导入环。
type Meta struct {
	ServerTime      time.Time `json:"server_time"`
	ResourceVersion int64     `json:"resource_version"`
}

type Envelope[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

// DimensionView 是单个维度的对外投影。它刻意把 raw_rate、两个置信度与最终
// 分数全部暴露：声望必须可解释，而不是一个不可追问的数字。
type DimensionView struct {
	Dimension          string  `json:"dimension"`
	SampleSize         int     `json:"sample_size"`
	PassedCount        int     `json:"passed_count"`
	EffectiveSample    float64 `json:"effective_sample"`
	RawRate            float64 `json:"raw_rate"`
	LifetimeConfidence float64 `json:"lifetime_confidence"`
	RecentConfidence   float64 `json:"recent_confidence"`
	Score              float64 `json:"score"`
	SampleSizeHint     string  `json:"sample_size_hint"`
	// Observed 为 false 表示零观测：该维度不参与总分，也不代表表现差。
	Observed bool `json:"observed"`
}

// ScoreCardView 是一条 v2 声望投影。OverallScore 为 nil 表示样本不足，
// 调用方必须显示"未验证"而不是确定性排名。
type ScoreCardView struct {
	Scope               string          `json:"scope"`
	AgentID             string          `json:"agent_id"`
	AgentVersionID      string          `json:"agent_version_id,omitempty"`
	Capability          string          `json:"capability,omitempty"`
	CanonicalRepository string          `json:"canonical_repository,omitempty"`
	AlgorithmVersion    string          `json:"algorithm_version"`
	EvaluatedAt         time.Time       `json:"evaluated_at"`
	OverallScore        *float64        `json:"overall_score"`
	SampleSize          int             `json:"sample_size"`
	SampleSizeHint      string          `json:"sample_size_hint"`
	Dimensions          []DimensionView `json:"dimensions"`
}

// AgentReputationView 一次性返回三层视图，避免调用方为了拼一张声望卡片
// 打三次请求。
type AgentReputationView struct {
	AgentID          string          `json:"agent_id"`
	AlgorithmVersion string          `json:"algorithm_version"`
	Lifetime         ScoreCardView   `json:"lifetime"`
	Versions         []ScoreCardView `json:"versions"`
	Capabilities     []ScoreCardView `json:"capabilities"`
}

// ScoreCardService 提供 v2 声望的只读视图。它不涉及租户：全局 Agent 的
// 声望是跨租户事实，视图里也刻意不含任何 sponsor 租户标识。
type ScoreCardService struct {
	cards            ScoreCardRepository
	algorithmVersion string
}

func NewScoreCardService(cards ScoreCardRepository, algorithmVersion string) (*ScoreCardService, error) {
	if cards == nil || algorithmVersion == "" {
		return nil, reputationdomain.ErrInvalidArgument
	}
	return &ScoreCardService{cards: cards, algorithmVersion: algorithmVersion}, nil
}

// GetAgentReputation 返回某个全局 Agent 的三层声望视图。
func (s *ScoreCardService) GetAgentReputation(ctx context.Context, agentID string) (Envelope[AgentReputationView], error) {
	var envelope Envelope[AgentReputationView]
	if agentID == "" {
		return envelope, reputationdomain.ErrInvalidArgument
	}
	cards, err := s.cards.ListByAgent(ctx, s.algorithmVersion, agentID)
	if err != nil {
		return envelope, err
	}

	view := AgentReputationView{
		AgentID:          agentID,
		AlgorithmVersion: s.algorithmVersion,
		// 没有任何投影时返回一张空白但结构完整的卡片：调用方拿到的永远是
		// "未验证"，而不是缺字段导致的隐式零分。
		Lifetime:     emptyScoreCardView(agentID, s.algorithmVersion),
		Versions:     []ScoreCardView{},
		Capabilities: []ScoreCardView{},
	}
	for _, card := range cards {
		item := toScoreCardView(card)
		switch card.Key.Scope {
		case reputationdomain.ScopeAgentLifetime:
			view.Lifetime = item
		case reputationdomain.ScopeAgentVersion:
			view.Versions = append(view.Versions, item)
		case reputationdomain.ScopeCapability:
			view.Capabilities = append(view.Capabilities, item)
		}
	}
	envelope.Data = view
	envelope.Meta = Meta{ServerTime: time.Now().UTC()}
	return envelope, nil
}

// GetSelfReputation 返回调用方 Agent 自己的声望。人类会话没有 Agent 身份，
// 因此被拒绝而不是回退到某个租户默认值。
func (s *ScoreCardService) GetSelfReputation(ctx context.Context, principal auth.Principal) (Envelope[AgentReputationView], error) {
	if principal.AgentID == "" {
		return Envelope[AgentReputationView]{}, coredomain.ErrForbidden
	}
	return s.GetAgentReputation(ctx, principal.AgentID)
}

func toScoreCardView(card reputationdomain.ScoreCard) ScoreCardView {
	dimensions := make([]DimensionView, 0, len(card.Dimensions))
	for _, score := range card.Dimensions {
		dimensions = append(dimensions, DimensionView{
			Dimension:          string(score.Dimension),
			SampleSize:         score.SampleSize,
			PassedCount:        score.PassedCount,
			EffectiveSample:    score.EffectiveSample,
			RawRate:            score.RawRate,
			LifetimeConfidence: score.LifetimeConfidence,
			RecentConfidence:   score.RecentConfidence,
			Score:              score.Score,
			SampleSizeHint:     score.SampleSizeHint,
			Observed:           score.Observed,
		})
	}
	return ScoreCardView{
		Scope:               string(card.Key.Scope),
		AgentID:             card.Key.AgentID,
		AgentVersionID:      card.Key.AgentVersionID,
		Capability:          card.Key.Capability,
		CanonicalRepository: card.Key.CanonicalRepository,
		AlgorithmVersion:    card.AlgorithmVersion,
		EvaluatedAt:         card.EvaluatedAt,
		OverallScore:        card.OverallScore,
		SampleSize:          card.SampleSize,
		SampleSizeHint:      card.SampleSizeHint,
		Dimensions:          dimensions,
	}
}

func emptyScoreCardView(agentID, algorithmVersion string) ScoreCardView {
	dimensions := make([]DimensionView, 0, len(reputationdomain.Dimensions))
	for _, dimension := range reputationdomain.Dimensions {
		dimensions = append(dimensions, DimensionView{
			Dimension:      string(dimension),
			SampleSizeHint: reputationdomain.SampleSizeHint(0),
		})
	}
	return ScoreCardView{
		Scope:            string(reputationdomain.ScopeAgentLifetime),
		AgentID:          agentID,
		AlgorithmVersion: algorithmVersion,
		SampleSizeHint:   reputationdomain.SampleSizeHint(0),
		Dimensions:       dimensions,
	}
}
