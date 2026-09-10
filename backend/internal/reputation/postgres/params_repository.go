package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ParamsRepository 读取版本化的算法参数。权重是数据不是 Go 常量：
// 历史投影必须能用当时的参数复算（doc §4.5）。
type ParamsRepository struct {
	pool *pgxpool.Pool
}

func NewParamsRepository(pool *pgxpool.Pool) *ParamsRepository {
	return &ParamsRepository{pool: pool}
}

var _ application.ParamsRepository = (*ParamsRepository)(nil)

// EnsureVersion 在参数行缺失时按给定参数创建它，已存在则原样保留。
//
// 部署方通过环境变量给出的半衰期/最小样本只在**首次**创建某个
// algorithm_version 时生效。已经算过声望的版本必须保持参数不变，否则
// 历史投影就无法用当时的参数复算——那正是 doc §4.5 禁止的原地改写。
func (r *ParamsRepository) EnsureVersion(ctx context.Context, params reputationdomain.Params) error {
	if err := params.Validate(); err != nil {
		return err
	}
	weights := make(map[string]float64, len(params.DimensionWeights))
	for dimension, weight := range params.DimensionWeights {
		weights[string(dimension)] = weight
	}
	encoded, err := json.Marshal(weights)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO reputation_algorithm_params (
			algorithm_version, dimension_weights, half_life_days, recent_weight_bps,
			wilson_z, prior_alpha, prior_beta, min_sample_for_score
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (algorithm_version) DO NOTHING`,
		params.AlgorithmVersion, encoded,
		params.HalfLife.Hours()/24, params.RecentWeightBps,
		params.WilsonZ, params.PriorAlpha, params.PriorBeta, params.MinSampleForScore)
	return err
}

func (r *ParamsRepository) Get(ctx context.Context, algorithmVersion string) (reputationdomain.Params, error) {
	if algorithmVersion == "" {
		return reputationdomain.Params{}, reputationdomain.ErrInvalidArgument
	}
	var (
		weights      []byte
		halfLifeDays float64
		params       reputationdomain.Params
	)
	err := r.pool.QueryRow(ctx, `
		SELECT dimension_weights, half_life_days, recent_weight_bps,
		       wilson_z, prior_alpha, prior_beta, min_sample_for_score
		FROM reputation_algorithm_params
		WHERE algorithm_version=$1`, algorithmVersion).Scan(
		&weights, &halfLifeDays, &params.RecentWeightBps,
		&params.WilsonZ, &params.PriorAlpha, &params.PriorBeta, &params.MinSampleForScore,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return reputationdomain.Params{}, reputationdomain.ErrNotFound
	}
	if err != nil {
		return reputationdomain.Params{}, err
	}

	decoded := map[string]float64{}
	if len(weights) > 0 {
		if err := json.Unmarshal(weights, &decoded); err != nil {
			return reputationdomain.Params{}, err
		}
	}
	params.AlgorithmVersion = algorithmVersion
	params.HalfLife = time.Duration(halfLifeDays * float64(24*time.Hour))
	params.DimensionWeights = make(map[reputationdomain.Dimension]float64, len(decoded))
	for name, weight := range decoded {
		dimension := reputationdomain.Dimension(name)
		if !reputationdomain.ValidDimension(dimension) {
			// 未知维度名说明参数行与代码版本不匹配，宁可报错也不静默丢弃。
			return reputationdomain.Params{}, reputationdomain.ErrInvalidArgument
		}
		params.DimensionWeights[dimension] = weight
	}
	if err := params.Validate(); err != nil {
		return reputationdomain.Params{}, err
	}
	return params, nil
}
