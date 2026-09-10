package application

import (
	"context"
	"time"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// FactSource 提供全部可用于重算的已验证交付事实。它必须是**确定性**的：
// 同一个数据库状态永远返回同一批事实（含顺序），否则重算无法逐字节复现。
type FactSource interface {
	// algorithmVersion 决定难度系数取哪一版：系数与维度权重同属评分参数，
	// 必须随版本冻结，否则历史投影无法用当时的参数复算。
	ListContributionFacts(ctx context.Context, algorithmVersion string) ([]ContributionFact, error)
	// LatestEventID 返回事实层当前的事件水位，供增量 worker 判断是否需要重算。
	LatestEventID(ctx context.Context) (int64, error)
}

// ParamsRepository 读取版本化的算法参数。权重是数据不是常量：历史投影
// 必须能用当时的参数复算。
type ParamsRepository interface {
	Get(ctx context.Context, algorithmVersion string) (reputationdomain.Params, error)
}

// ScoreCardRepository 存储 v2 投影。ReplaceAlgorithm 必须在单个事务里
// 先删除该 algorithm_version 的全部行再插入，其他版本不受影响。
type ScoreCardRepository interface {
	ReplaceAlgorithm(ctx context.Context, algorithmVersion string, cards []reputationdomain.ScoreCard) error
	Get(ctx context.Context, algorithmVersion string, key reputationdomain.ScoreKey) (*reputationdomain.ScoreCard, error)
	ListByAgent(ctx context.Context, algorithmVersion, agentID string) ([]reputationdomain.ScoreCard, error)
	// MaxLatestEventID 返回已落库投影的事件水位；无投影时返回 -1。
	MaxLatestEventID(ctx context.Context, algorithmVersion string) (int64, error)
}

// Rebuilder 从不可变事实全量重算 v2 声望投影。
type Rebuilder struct {
	facts  FactSource
	params ParamsRepository
	cards  ScoreCardRepository
}

func NewRebuilder(facts FactSource, params ParamsRepository, cards ScoreCardRepository) (*Rebuilder, error) {
	if facts == nil || params == nil || cards == nil {
		return nil, reputationdomain.ErrInvalidArgument
	}
	return &Rebuilder{facts: facts, params: params, cards: cards}, nil
}

// RebuildResult 描述一次重算的产出，供 worker 与 admin 端点回报。
type RebuildResult struct {
	AlgorithmVersion string    `json:"algorithm_version"`
	EvaluatedAt      time.Time `json:"evaluated_at"`
	ProjectionCount  int       `json:"projection_count"`
	FactCount        int       `json:"fact_count"`
	LatestEventID    int64     `json:"latest_event_id"`
}

// Rebuild 在固定的 evaluatedAt 下全量重算。
//
// evaluatedAt 必须由调用方显式给出：引入 180 天衰减之后，"可完整重算"
// 只在固定评估时刻成立。同一个 evaluatedAt 重跑必须得到逐字节相同的投影。
func (r *Rebuilder) Rebuild(ctx context.Context, algorithmVersion string, evaluatedAt time.Time) (RebuildResult, error) {
	if algorithmVersion == "" {
		return RebuildResult{}, reputationdomain.ErrInvalidArgument
	}
	if evaluatedAt.IsZero() {
		return RebuildResult{}, reputationdomain.ErrInvalidEvaluatedAt
	}
	params, err := r.params.Get(ctx, algorithmVersion)
	if err != nil {
		return RebuildResult{}, err
	}
	scorer, err := NewScorer(params)
	if err != nil {
		return RebuildResult{}, err
	}
	facts, err := r.facts.ListContributionFacts(ctx, algorithmVersion)
	if err != nil {
		return RebuildResult{}, err
	}
	cards, err := scorer.Score(facts, evaluatedAt.UTC())
	if err != nil {
		return RebuildResult{}, err
	}
	if err := r.cards.ReplaceAlgorithm(ctx, algorithmVersion, cards); err != nil {
		return RebuildResult{}, err
	}

	result := RebuildResult{
		AlgorithmVersion: algorithmVersion,
		EvaluatedAt:      evaluatedAt.UTC(),
		ProjectionCount:  len(cards),
		FactCount:        len(facts),
	}
	for _, card := range cards {
		if card.LatestEventID > result.LatestEventID {
			result.LatestEventID = card.LatestEventID
		}
	}
	return result, nil
}
