package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reputationpostgres "agentguild.dev/agentguild/backend/internal/reputation/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// pinnedEvaluatedAt 是固定的评估时刻。引入 180 天衰减之后，"可完整重算"
// 只在固定评估时刻成立，因此所有断言都钉在这一刻上。
var pinnedEvaluatedAt = time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)

const v2Version = reputationdomain.DefaultAlgorithmVersionV2

// seedV2Facts 种入一个全局 Agent、两个版本，以及若干条已核验交付事实。
// 全部走真实表与真实外键，避免测试与生产 schema 漂移。
func seedV2Facts(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ('agent-global', 'agent-global', 'Global Agent', 'active');
		INSERT INTO agent_identity_versions (id, agent_id, version_number, status, runtime, model)
		VALUES
			('version-1', 'agent-global', 1, 'retired', 'pi', 'gpt-5'),
			('version-2', 'agent-global', 2, 'active', 'pi', 'gpt-5');
		UPDATE agent_identities SET current_version_id='version-2' WHERE id='agent-global';`)
	require.NoError(t, err)

	for i := 1; i <= 6; i++ {
		seedDelivery(t, db, deliverySpec{
			index: i, versionID: "version-1", passed: true,
		})
	}
}

type deliverySpec struct {
	index     int
	versionID string
	passed    bool
}

// seedDelivery 写入一次完整交付：task → execution → public task projection →
// contribution → 事件 → criterion 结果。
func seedDelivery(t *testing.T, db *pgxpool.Pool, spec deliverySpec) {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", spec.index)
	taskID := "task-" + suffix
	executionID := "execution-" + suffix
	contributionID := "contribution-" + suffix
	commit := fmt.Sprintf("%040d", spec.index)
	submittedAt := pinnedEvaluatedAt.Add(-time.Duration(spec.index) * 24 * time.Hour)

	// pgx 不允许在带参数的 prepared statement 里塞多条语句，逐条写入。
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (tenant_id, id, publisher_agent_version_id, type, title, problem, deadline, status)
		VALUES ('tenant-sponsor', $1, 'publisher-version', 'bug', 'Fix widget', 'Widget fails', $2, 'open')`,
		taskID, pinnedEvaluatedAt.Add(24*time.Hour))
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_id, agent_version_id, status, lease_generation, submitted_at
		) VALUES ('tenant-sponsor', $1, $2, 'agent-global', $3, 'accepted', 0, $4)`,
		executionID, taskID, spec.versionID, submittedAt)
	require.NoError(t, err)

	// 规格里有两条必需标准，其中 AC-SEC 绑定 security_scan。
	// 账本里刻意没有它的结果——security 维度必须保持零样本。
	_, err = db.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id, canonical_repository,
			source_issue_url, issue_revision, base_commit, title, summary, problem_diagnosis,
			impact, proposed_solution, acceptance_criteria, quality_level, published_at,
			difficulty_class
		) VALUES (
			$1, 'tenant-sponsor', $2, 'specification-1', 'acme/widgets',
			'https://github.com/acme/widgets/issues/1', 'rev-1', $3, 'Fix widget', 'summary',
			'diagnosis', 'impact', 'solution',
			'[{"id":"AC-1","statement":"tests pass","critical":true,"verifier_kind":"command","verifier_ref":"public_tests"},
			  {"id":"AC-SEC","statement":"no secrets","critical":true,"verifier_kind":"command","verifier_ref":"security_scan"}]'::jsonb,
			'standard', $4, 'substantial'
		)`,
		"projection-"+suffix, taskID, commit, submittedAt)
	require.NoError(t, err)

	_, err = db.Exec(ctx, `
		INSERT INTO contributions (
			id, resource_tenant_id, agent_id, agent_version_id, task_id, execution_id,
			task_specification_version_id, canonical_repository, issue_number, issue_url,
			provider, pull_request_number, pull_request_url, commit_sha,
			attribution_status, outcome, created_at
		) VALUES (
			$1, 'tenant-sponsor', 'agent-global', $2, $3, $4, 'specification-1', 'acme/widgets',
			$5, 'https://github.com/acme/widgets/issues/1', 'github', $5,
			'https://github.com/acme/widgets/pull/1', $6, 'verified', 'merged', $7
		)`,
		contributionID, spec.versionID, taskID, executionID, int64(spec.index), commit, submittedAt)
	require.NoError(t, err)

	appendEvent(t, db, contributionID, "ci", "ci_passed", commit, "delivery-ci-"+suffix, submittedAt)
	appendEvent(t, db, contributionID, "merged", "merged", commit, "delivery-merged-"+suffix, submittedAt)

	_, err = db.Exec(ctx, `
		INSERT INTO execution_criterion_results (
			resource_tenant_id, task_id, execution_id, criterion_id, critical, verifier_kind,
			passed, source_kind, source_id, verified_by, observed_at
		) VALUES
			('tenant-sponsor', $1, $2, 'AC-1', true, 'command', $3, 'validation_job', $4, 'validation_worker', $5)`,
		taskID, executionID, spec.passed, "job-"+suffix, submittedAt)
	require.NoError(t, err)
}

func appendEvent(t *testing.T, db *pgxpool.Pool, contributionID, eventType, outcome, commit, delivery string, at time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO contribution_events (
			contribution_id, provider, provider_delivery_id, event_type, outcome,
			commit_sha, payload, occurred_at
		) VALUES ($1, 'github', $2, $3, $4, $5, '{}'::jsonb, $6)
		ON CONFLICT DO NOTHING`,
		contributionID, delivery, eventType, outcome, commit, at)
	require.NoError(t, err)
}

func newRebuilder(t *testing.T, db *pgxpool.Pool) (*reputationapp.Rebuilder, reputationapp.ScoreCardRepository) {
	t.Helper()
	cards := reputationpostgres.NewScoreCardRepository(db)
	rebuilder, err := reputationapp.NewRebuilder(
		reputationpostgres.NewFactSource(db),
		reputationpostgres.NewParamsRepository(db),
		cards,
	)
	require.NoError(t, err)
	return rebuilder, cards
}

// 删光投影后在固定 evaluatedAt 下重算，必须逐字节复现。
func TestRebuildReproducesProjectionsByteForByte(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, cards := newRebuilder(t, db)

	first, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	require.Positive(t, first.ProjectionCount)
	require.Equal(t, 6, first.FactCount)
	before, err := cards.ListByAgent(ctx, v2Version, "agent-global")
	require.NoError(t, err)
	require.NotEmpty(t, before)

	// 删光投影，再在同一个评估时刻重算。
	_, err = db.Exec(ctx, `DELETE FROM agent_reputation_projections WHERE algorithm_version=$1`, v2Version)
	require.NoError(t, err)
	var dimensionRows int
	require.NoError(t, db.QueryRow(ctx, `SELECT COUNT(*) FROM agent_reputation_dimension_scores`).Scan(&dimensionRows))
	require.Zero(t, dimensionRows, "维度行必须随 header 级联删除")

	second, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	after, err := cards.ListByAgent(ctx, v2Version, "agent-global")
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, before, after, "固定评估时刻下的重算必须逐字节复现")
}

// 同一事件重复投递不得改变分数：账本按 provider 幂等键去重，
// 而事实映射只看每个 outcome 是否出现过。
func TestReplayedEventsDoNotChangeScores(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, cards := newRebuilder(t, db)

	_, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	before, err := cards.ListByAgent(ctx, v2Version, "agent-global")
	require.NoError(t, err)

	// 同一条 merged 事实以新的 delivery id 再投一次。
	appendEvent(t, db, "contribution-1", "merged", "merged", fmt.Sprintf("%040d", 1),
		"delivery-merged-1-redelivery", pinnedEvaluatedAt.Add(-24*time.Hour))

	_, err = rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	after, err := cards.ListByAgent(ctx, v2Version, "agent-global")
	require.NoError(t, err)

	require.Equal(t, stripWatermarks(before), stripWatermarks(after),
		"重复投递同一事实不得改变任何分数")
}

// stripWatermarks 去掉事件水位：水位本来就会随新事件行前进，
// 需要断言的是分数不变。
func stripWatermarks(cards []reputationdomain.ScoreCard) []reputationdomain.ScoreCard {
	stripped := make([]reputationdomain.ScoreCard, 0, len(cards))
	for _, card := range cards {
		card.LatestEventID = 0
		stripped = append(stripped, card)
	}
	return stripped
}

// 新建 Agent Version 的样本归零，但 agent lifetime 不丢。
func TestNewAgentVersionResetsSamplesWithoutLosingLifetime(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, cards := newRebuilder(t, db)

	_, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	lifetimeBefore, err := cards.Get(ctx, v2Version, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentLifetime, AgentID: "agent-global",
	})
	require.NoError(t, err)
	require.Equal(t, 6, lifetimeBefore.SampleSize)
	require.NotNil(t, lifetimeBefore.OverallScore)

	// 新版本上产生第一次交付。
	seedDelivery(t, db, deliverySpec{index: 7, versionID: "version-2", passed: true})
	_, err = rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)

	newVersion, err := cards.Get(ctx, v2Version, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentVersion, AgentID: "agent-global", AgentVersionID: "version-2",
	})
	require.NoError(t, err)
	require.Equal(t, 1, newVersion.SampleSize, "新版本不继承旧版本的质量样本")
	require.Nil(t, newVersion.OverallScore, "样本不足时不得给出确定性分数")
	require.Equal(t, "unverified", newVersion.SampleSizeHint)

	lifetimeAfter, err := cards.Get(ctx, v2Version, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentLifetime, AgentID: "agent-global",
	})
	require.NoError(t, err)
	require.Equal(t, 7, lifetimeAfter.SampleSize, "agent lifetime 必须保留全部历史样本")
	require.NotNil(t, lifetimeAfter.OverallScore)
}

// security 维度零行 = 零样本，绝不是通过。规格里有一条绑定 security_scan
// 的必需标准，但账本里没有任何安全结果。
func TestSecurityDimensionStaysUnverifiedWithoutEvidence(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, cards := newRebuilder(t, db)

	_, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)
	lifetime, err := cards.Get(ctx, v2Version, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentLifetime, AgentID: "agent-global",
	})
	require.NoError(t, err)

	require.Len(t, lifetime.Dimensions, 7)
	for _, dimension := range lifetime.Dimensions {
		if dimension.Dimension != reputationdomain.DimensionSecurity {
			continue
		}
		require.Equal(t, 0, dimension.SampleSize)
		require.False(t, dimension.Observed)
		require.Zero(t, dimension.Score)
		require.Equal(t, "unverified", dimension.SampleSizeHint)
	}
}

// ReplaceAlgorithm 只删自己那个 algorithm_version，历史版本原样保留。
func TestReplaceAlgorithmLeavesOtherVersionsIntact(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, cards := newRebuilder(t, db)

	// 造一个"历史算法版本"，参数与当前版本相同但版本号不同。
	_, err := db.Exec(ctx, `
		INSERT INTO reputation_algorithm_params (
			algorithm_version, dimension_weights, half_life_days, recent_weight_bps,
			wilson_z, prior_alpha, prior_beta, min_sample_for_score
		) SELECT '2026-01-01-v1', dimension_weights, half_life_days, recent_weight_bps,
		         wilson_z, prior_alpha, prior_beta, min_sample_for_score
		  FROM reputation_algorithm_params WHERE algorithm_version=$1`, v2Version)
	require.NoError(t, err)

	_, err = rebuilder.Rebuild(ctx, "2026-01-01-v1", pinnedEvaluatedAt)
	require.NoError(t, err)
	legacy, err := cards.ListByAgent(ctx, "2026-01-01-v1", "agent-global")
	require.NoError(t, err)
	require.NotEmpty(t, legacy)

	_, err = rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt.Add(48*time.Hour))
	require.NoError(t, err)

	stillThere, err := cards.ListByAgent(ctx, "2026-01-01-v1", "agent-global")
	require.NoError(t, err)
	require.Equal(t, legacy, stillThere, "算法升级不得原地改写历史版本的投影")
}

// v1 的 reputation_projections 与 v2 的表并行存在，互不影响。
func TestV1ProjectionTableIsUntouched(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedV2Facts(t, db)
	ctx := context.Background()
	rebuilder, _ := newRebuilder(t, db)

	_, err := rebuilder.Rebuild(ctx, v2Version, pinnedEvaluatedAt)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow(ctx, `SELECT COUNT(*) FROM reputation_projections`).Scan(&count))
	require.Zero(t, count, "v2 重算不得写入 v1 的表")
}

func TestParamsRepositoryRejectsUnknownAlgorithmVersion(t *testing.T) {
	db := testdb.StartPostgres(t)
	_, err := reputationpostgres.NewParamsRepository(db).Get(context.Background(), "no-such-version")
	require.ErrorIs(t, err, reputationdomain.ErrNotFound)
}

func TestDifficultyMultiplierIsBoundedByTheClassTable(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO task_difficulty_classes (algorithm_version, class, multiplier)
		VALUES ($1, 'absurd', 5.0)`, reputationdomain.DefaultAlgorithmVersionV2)
	require.Error(t, err, "难度系数必须被约束在 [0.75, 1.50]")
}

// 难度系数与维度权重同属评分参数，必须随 algorithm_version 冻结：否则调整
// 一次系数就会静默改变所有历史投影的重算结果，等于原地改写历史。
func TestDifficultyClassesAreScopedToAnAlgorithmVersion(t *testing.T) {
	db := testdb.StartPostgres(t)
	ctx := context.Background()

	// 同名 class 可以在不同版本下有不同系数，互不干扰。
	_, err := db.Exec(ctx, `
		INSERT INTO reputation_algorithm_params (
			algorithm_version, dimension_weights, half_life_days, recent_weight_bps,
			wilson_z, prior_alpha, prior_beta, min_sample_for_score
		) VALUES ('test-v3', '{}'::jsonb, 90, 5000, 1.96, 0.5, 0.5, 5)`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO task_difficulty_classes (algorithm_version, class, multiplier)
		VALUES ('test-v3', 'complex', 0.80)`)
	require.NoError(t, err)

	var v2, v3 float64
	require.NoError(t, db.QueryRow(ctx, `
		SELECT multiplier FROM task_difficulty_classes
		WHERE algorithm_version=$1 AND class='complex'`,
		reputationdomain.DefaultAlgorithmVersionV2).Scan(&v2))
	require.NoError(t, db.QueryRow(ctx, `
		SELECT multiplier FROM task_difficulty_classes
		WHERE algorithm_version='test-v3' AND class='complex'`).Scan(&v3))
	require.Equal(t, 1.50, v2)
	require.Equal(t, 0.80, v3, "调整新版本的系数不得改动既有版本")

	// 没有对应参数行的版本不得插入分级，避免出现无主的孤儿系数。
	_, err = db.Exec(ctx, `
		INSERT INTO task_difficulty_classes (algorithm_version, class, multiplier)
		VALUES ('never-created', 'standard', 1.0)`)
	require.Error(t, err)
}
