package application

import (
	"time"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// Scorer 把一批已验证交付事实投影成三层视图（doc §4.2）：
// agent_lifetime / agent_version / capability。
//
// 它是纯函数：同一批事实 + 同一份参数 + 同一个 evaluatedAt 永远产生
// 逐字节相同的输出。所有时间语义都来自 evaluatedAt，不读时钟。
type Scorer struct {
	params reputationdomain.Params
}

func NewScorer(params reputationdomain.Params) (*Scorer, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &Scorer{params: params}, nil
}

// Params 暴露本次评分使用的参数，供存储层记录算法版本。
func (s *Scorer) Params() reputationdomain.Params { return s.params }

// Score 生成全部投影。返回值按 ScoreKey 稳定排序。
func (s *Scorer) Score(facts []ContributionFact, evaluatedAt time.Time) ([]reputationdomain.ScoreCard, error) {
	if evaluatedAt.IsZero() {
		return nil, reputationdomain.ErrInvalidEvaluatedAt
	}
	groups := make(map[reputationdomain.ScoreKey]*scoreGroup)
	for _, fact := range facts {
		if fact.AgentID == "" || fact.AgentVersionID == "" {
			return nil, reputationdomain.ErrInvalidFact
		}
		observations := fact.Observations()
		latest := fact.LatestEventID()

		// agent_lifetime：跨版本、跨仓库的总体表现。
		groupFor(groups, reputationdomain.ScoreKey{
			Scope:   reputationdomain.ScopeAgentLifetime,
			AgentID: fact.AgentID,
		}).add(observations, latest)

		// agent_version：版本之间不继承质量样本，新版本从零开始。
		groupFor(groups, reputationdomain.ScoreKey{
			Scope:          reputationdomain.ScopeAgentVersion,
			AgentID:        fact.AgentID,
			AgentVersionID: fact.AgentVersionID,
		}).add(observations, latest)

		// capability：能力 × 仓库。两者都为空时无法构成合法键，跳过。
		if fact.Capability != "" || fact.CanonicalRepository != "" {
			groupFor(groups, reputationdomain.ScoreKey{
				Scope:               reputationdomain.ScopeCapability,
				AgentID:             fact.AgentID,
				Capability:          fact.Capability,
				CanonicalRepository: fact.CanonicalRepository,
			}).add(observations, latest)
		}
	}

	cards := make([]reputationdomain.ScoreCard, 0, len(groups))
	for key, group := range groups {
		card, err := reputationdomain.Score(key, group.sampleSize, group.observations, s.params, evaluatedAt)
		if err != nil {
			return nil, err
		}
		card.LatestEventID = group.latestEventID
		cards = append(cards, card)
	}
	reputationdomain.SortScoreCards(cards)
	return cards, nil
}

// scoreGroup 累积一个投影键上的观测。sampleSize 按**交付次数**计数，
// 而不是观测条数——否则一次交付的多条验收标准就能虚高样本量。
type scoreGroup struct {
	observations  []reputationdomain.Observation
	sampleSize    int
	latestEventID int64
}

func (g *scoreGroup) add(observations []reputationdomain.Observation, latestEventID int64) {
	g.observations = append(g.observations, observations...)
	g.sampleSize++
	if latestEventID > g.latestEventID {
		g.latestEventID = latestEventID
	}
}

func groupFor(groups map[reputationdomain.ScoreKey]*scoreGroup, key reputationdomain.ScoreKey) *scoreGroup {
	group := groups[key]
	if group == nil {
		group = &scoreGroup{}
		groups[key] = group
	}
	return group
}
