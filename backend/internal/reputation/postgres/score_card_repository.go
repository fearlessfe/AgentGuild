package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScoreCardRepository 存储 v2 声望投影。v1 的 reputation_projections 由
// projection_repository.go 独立维护，两者互不影响。
type ScoreCardRepository struct {
	pool *pgxpool.Pool
}

func NewScoreCardRepository(pool *pgxpool.Pool) application.ScoreCardRepository {
	return &ScoreCardRepository{pool: pool}
}

// ReplaceAlgorithm 在**单个事务**内删除该 algorithm_version 的全部行再插入。
// 删除范围严格限定在这一个版本上：其他 algorithm_version 的历史投影不受
// 影响，这正是"算法升级不原地改写历史"的落地方式（doc §4.5）。
// 维度行通过 ON DELETE CASCADE 随 header 一并删除。
func (r *ScoreCardRepository) ReplaceAlgorithm(ctx context.Context, algorithmVersion string, cards []reputationdomain.ScoreCard) error {
	if algorithmVersion == "" {
		return reputationdomain.ErrInvalidArgument
	}
	for _, card := range cards {
		if card.AlgorithmVersion != algorithmVersion || !card.Key.Valid() {
			return reputationdomain.ErrInvalidArgument
		}
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`DELETE FROM agent_reputation_projections WHERE algorithm_version=$1`, algorithmVersion); err != nil {
		return err
	}
	for _, card := range cards {
		if err := insertScoreCard(ctx, tx, card); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func insertScoreCard(ctx context.Context, tx pgx.Tx, card reputationdomain.ScoreCard) error {
	key := card.Key
	if _, err := tx.Exec(ctx, `
		INSERT INTO agent_reputation_projections (
			algorithm_version, scope, agent_id, agent_version_id, capability,
			canonical_repository, evaluated_at, overall_score, sample_size,
			sample_size_hint, latest_event_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		card.AlgorithmVersion, string(key.Scope), key.AgentID, key.AgentVersionID,
		key.Capability, key.CanonicalRepository, card.EvaluatedAt, card.OverallScore,
		card.SampleSize, card.SampleSizeHint, card.LatestEventID,
	); err != nil {
		return err
	}
	for _, score := range card.Dimensions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO agent_reputation_dimension_scores (
				algorithm_version, scope, agent_id, agent_version_id, capability,
				canonical_repository, dimension, sample_size, passed_count,
				effective_sample, effective_passed, raw_rate, lifetime_confidence,
				recent_confidence, score, sample_size_hint, observed
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			card.AlgorithmVersion, string(key.Scope), key.AgentID, key.AgentVersionID,
			key.Capability, key.CanonicalRepository, string(score.Dimension),
			score.SampleSize, score.PassedCount, score.EffectiveSample, score.EffectivePassed,
			score.RawRate, score.LifetimeConfidence, score.RecentConfidence, score.Score,
			score.SampleSizeHint, score.Observed,
		); err != nil {
			return err
		}
	}
	return nil
}

const scoreCardColumns = `
	algorithm_version, scope, agent_id, agent_version_id, capability,
	canonical_repository, evaluated_at, overall_score, sample_size,
	sample_size_hint, latest_event_id`

func (r *ScoreCardRepository) Get(ctx context.Context, algorithmVersion string, key reputationdomain.ScoreKey) (*reputationdomain.ScoreCard, error) {
	if algorithmVersion == "" || !key.Valid() {
		return nil, reputationdomain.ErrInvalidArgument
	}
	rows, err := r.pool.Query(ctx, `
		SELECT`+scoreCardColumns+`
		FROM agent_reputation_projections
		WHERE algorithm_version=$1 AND scope=$2 AND agent_id=$3
		  AND agent_version_id=$4 AND capability=$5 AND canonical_repository=$6`,
		algorithmVersion, string(key.Scope), key.AgentID,
		key.AgentVersionID, key.Capability, key.CanonicalRepository)
	if err != nil {
		return nil, err
	}
	cards, err := scanScoreCards(rows)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, reputationdomain.ErrNotFound
	}
	if err := r.attachDimensions(ctx, algorithmVersion, cards); err != nil {
		return nil, err
	}
	return &cards[0], nil
}

func (r *ScoreCardRepository) ListByAgent(ctx context.Context, algorithmVersion, agentID string) ([]reputationdomain.ScoreCard, error) {
	if algorithmVersion == "" || agentID == "" {
		return nil, reputationdomain.ErrInvalidArgument
	}
	rows, err := r.pool.Query(ctx, `
		SELECT`+scoreCardColumns+`
		FROM agent_reputation_projections
		WHERE algorithm_version=$1 AND agent_id=$2
		ORDER BY scope, agent_version_id, capability, canonical_repository`,
		algorithmVersion, agentID)
	if err != nil {
		return nil, err
	}
	cards, err := scanScoreCards(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachDimensions(ctx, algorithmVersion, cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// MaxLatestEventID 返回已落库投影的事件水位。没有任何投影时返回 -1，
// 让 worker 能区分"从未算过"与"算过但事实层为空"。
func (r *ScoreCardRepository) MaxLatestEventID(ctx context.Context, algorithmVersion string) (int64, error) {
	if algorithmVersion == "" {
		return 0, reputationdomain.ErrInvalidArgument
	}
	var watermark *int64
	err := r.pool.QueryRow(ctx, `
		SELECT MAX(latest_event_id) FROM agent_reputation_projections
		WHERE algorithm_version=$1`, algorithmVersion).Scan(&watermark)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if watermark == nil {
		return -1, nil
	}
	return *watermark, nil
}

func scanScoreCards(rows pgx.Rows) ([]reputationdomain.ScoreCard, error) {
	defer rows.Close()
	cards := make([]reputationdomain.ScoreCard, 0, 8)
	for rows.Next() {
		var (
			card  reputationdomain.ScoreCard
			scope string
		)
		if err := rows.Scan(
			&card.AlgorithmVersion, &scope, &card.Key.AgentID, &card.Key.AgentVersionID,
			&card.Key.Capability, &card.Key.CanonicalRepository, &card.EvaluatedAt,
			&card.OverallScore, &card.SampleSize, &card.SampleSizeHint, &card.LatestEventID,
		); err != nil {
			return nil, err
		}
		card.Key.Scope = reputationdomain.ScopeKind(scope)
		card.EvaluatedAt = card.EvaluatedAt.UTC()
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

// attachDimensions 一次性取回全部维度行并按 header 归位。维度行永远是七行，
// 缺行说明数据被外部改动过，这里补齐成零观测而不是留空。
func (r *ScoreCardRepository) attachDimensions(ctx context.Context, algorithmVersion string, cards []reputationdomain.ScoreCard) error {
	if len(cards) == 0 {
		return nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT scope, agent_id, agent_version_id, capability, canonical_repository,
		       dimension, sample_size, passed_count, effective_sample, effective_passed,
		       raw_rate, lifetime_confidence, recent_confidence, score,
		       sample_size_hint, observed
		FROM agent_reputation_dimension_scores
		WHERE algorithm_version=$1 AND agent_id = ANY($2)`,
		algorithmVersion, agentIDsOf(cards))
	if err != nil {
		return err
	}
	defer rows.Close()

	byKey := make(map[reputationdomain.ScoreKey]map[reputationdomain.Dimension]reputationdomain.DimensionScore)
	for rows.Next() {
		var (
			key       reputationdomain.ScoreKey
			scope     string
			dimension string
			score     reputationdomain.DimensionScore
		)
		if err := rows.Scan(&scope, &key.AgentID, &key.AgentVersionID, &key.Capability,
			&key.CanonicalRepository, &dimension, &score.SampleSize, &score.PassedCount,
			&score.EffectiveSample, &score.EffectivePassed, &score.RawRate,
			&score.LifetimeConfidence, &score.RecentConfidence, &score.Score,
			&score.SampleSizeHint, &score.Observed); err != nil {
			return err
		}
		key.Scope = reputationdomain.ScopeKind(scope)
		score.Dimension = reputationdomain.Dimension(dimension)
		if byKey[key] == nil {
			byKey[key] = make(map[reputationdomain.Dimension]reputationdomain.DimensionScore, len(reputationdomain.Dimensions))
		}
		byKey[key][score.Dimension] = score
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range cards {
		stored := byKey[cards[i].Key]
		dimensions := make([]reputationdomain.DimensionScore, 0, len(reputationdomain.Dimensions))
		for _, dimension := range reputationdomain.Dimensions {
			score, found := stored[dimension]
			if !found {
				score = reputationdomain.DimensionScore{
					Dimension:      dimension,
					SampleSizeHint: reputationdomain.SampleSizeHint(0),
				}
			}
			dimensions = append(dimensions, score)
		}
		cards[i].Dimensions = dimensions
	}
	return nil
}

func agentIDsOf(cards []reputationdomain.ScoreCard) []string {
	seen := make(map[string]struct{}, len(cards))
	ids := make([]string, 0, len(cards))
	for _, card := range cards {
		if _, found := seen[card.Key.AgentID]; found {
			continue
		}
		seen[card.Key.AgentID] = struct{}{}
		ids = append(ids, card.Key.AgentID)
	}
	return ids
}
