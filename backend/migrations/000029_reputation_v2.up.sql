-- 声望 v2：七维、置信下界、时间衰减、算法版本化。
--
-- v1 的 reputation_projections 原样保留并继续服务 /v1/reputation。
-- 本迁移新增一族并行的投影表，主键含 algorithm_version 与 scope，
-- 因此算法升级只会写入新的版本行，绝不原地改写历史（doc §4.5）。

-- 算法参数是版本化数据，不是 Go 常量：历史行必须能用当时的参数复算。
CREATE TABLE reputation_algorithm_params (
    algorithm_version TEXT PRIMARY KEY,
    -- 维度权重，键必须是七个维度名之一；缺席维度权重视为 0。
    dimension_weights JSONB NOT NULL DEFAULT '{}'::jsonb,
    half_life_days DOUBLE PRECISION NOT NULL,
    recent_weight_bps INT NOT NULL,
    wilson_z DOUBLE PRECISION NOT NULL,
    prior_alpha DOUBLE PRECISION NOT NULL,
    prior_beta DOUBLE PRECISION NOT NULL,
    min_sample_for_score INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT reputation_algorithm_params_version_nonempty CHECK (algorithm_version <> ''),
    CONSTRAINT reputation_algorithm_params_weights_object CHECK (
        jsonb_typeof(dimension_weights) = 'object'
    ),
    CONSTRAINT reputation_algorithm_params_half_life_positive CHECK (half_life_days > 0),
    CONSTRAINT reputation_algorithm_params_recent_weight_valid CHECK (
        recent_weight_bps BETWEEN 0 AND 10000
    ),
    CONSTRAINT reputation_algorithm_params_wilson_z_positive CHECK (wilson_z > 0),
    CONSTRAINT reputation_algorithm_params_priors_positive CHECK (
        prior_alpha > 0 AND prior_beta > 0
    ),
    CONSTRAINT reputation_algorithm_params_min_sample_valid CHECK (min_sample_for_score >= 0)
);

-- 投影 header。主键含 algorithm_version 与 scope，三层视图共表存放。
-- 刻意不存 calculated_at：evaluated_at 是唯一的时间基准，
-- 任何随时钟变化的列都会让"逐字节复现"失去意义。
CREATE TABLE agent_reputation_projections (
    algorithm_version TEXT NOT NULL,
    scope TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL DEFAULT '',
    capability TEXT NOT NULL DEFAULT '',
    canonical_repository TEXT NOT NULL DEFAULT '',
    evaluated_at TIMESTAMPTZ NOT NULL,
    -- 样本不足时为 NULL：低样本不显示确定性排名（doc §4.4）。
    overall_score DOUBLE PRECISION,
    sample_size INT NOT NULL,
    sample_size_hint TEXT NOT NULL,
    latest_event_id BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (
        algorithm_version, scope, agent_id,
        agent_version_id, capability, canonical_repository
    ),
    CONSTRAINT agent_reputation_projections_params_fk
        FOREIGN KEY (algorithm_version)
        REFERENCES reputation_algorithm_params (algorithm_version),
    CONSTRAINT agent_reputation_projections_agent_fk
        FOREIGN KEY (agent_id) REFERENCES agent_identities (id),
    CONSTRAINT agent_reputation_projections_scope_valid CHECK (
        scope IN ('agent_lifetime', 'agent_version', 'capability')
    ),
    CONSTRAINT agent_reputation_projections_sample_hint_valid CHECK (
        sample_size_hint IN ('unverified', 'low', 'medium', 'high')
    ),
    CONSTRAINT agent_reputation_projections_sample_size_valid CHECK (sample_size >= 0),
    CONSTRAINT agent_reputation_projections_latest_event_valid CHECK (latest_event_id >= 0),
    CONSTRAINT agent_reputation_projections_score_range CHECK (
        overall_score IS NULL OR overall_score BETWEEN 0 AND 1
    ),
    -- 未验证的投影绝不能带确定性分数。
    CONSTRAINT agent_reputation_projections_unverified_has_no_score CHECK (
        sample_size_hint <> 'unverified' OR overall_score IS NULL
    ),
    -- scope 决定哪些维度键必须为空，避免同一行被两种口径解释。
    CONSTRAINT agent_reputation_projections_scope_keys CHECK (
        (scope = 'agent_lifetime'
            AND agent_version_id = '' AND capability = '' AND canonical_repository = '')
        OR (scope = 'agent_version'
            AND agent_version_id <> '' AND capability = '' AND canonical_repository = '')
        OR (scope = 'capability' AND agent_version_id = '')
    )
);

CREATE INDEX agent_reputation_projections_agent_lookup
    ON agent_reputation_projections (agent_id, algorithm_version, scope);

-- 每维一行。raw_rate / lifetime_confidence / recent_confidence / score 全部
-- 保留，让"为什么是这个分数"可以逐项解释而不是只给一个数字。
CREATE TABLE agent_reputation_dimension_scores (
    algorithm_version TEXT NOT NULL,
    scope TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_version_id TEXT NOT NULL DEFAULT '',
    capability TEXT NOT NULL DEFAULT '',
    canonical_repository TEXT NOT NULL DEFAULT '',
    dimension TEXT NOT NULL,
    sample_size INT NOT NULL,
    passed_count INT NOT NULL,
    effective_sample DOUBLE PRECISION NOT NULL,
    effective_passed DOUBLE PRECISION NOT NULL,
    raw_rate DOUBLE PRECISION NOT NULL,
    lifetime_confidence DOUBLE PRECISION NOT NULL,
    recent_confidence DOUBLE PRECISION NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    sample_size_hint TEXT NOT NULL,
    -- observed=false 表示零观测：该维度被排除在总分之外并重新归一化，
    -- 而不是按 0 分计入。"没做过安全评审"不等于"安全表现差"。
    observed BOOLEAN NOT NULL,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    PRIMARY KEY (
        algorithm_version, scope, agent_id,
        agent_version_id, capability, canonical_repository, dimension
    ),
    CONSTRAINT agent_reputation_dimension_scores_header_fk
        FOREIGN KEY (
            algorithm_version, scope, agent_id,
            agent_version_id, capability, canonical_repository
        )
        REFERENCES agent_reputation_projections (
            algorithm_version, scope, agent_id,
            agent_version_id, capability, canonical_repository
        )
        ON DELETE CASCADE,
    CONSTRAINT agent_reputation_dimension_scores_dimension_valid CHECK (
        dimension IN (
            'correctness', 'reliability', 'reviewability',
            'maintainability', 'security', 'collaboration', 'impact'
        )
    ),
    CONSTRAINT agent_reputation_dimension_scores_sample_hint_valid CHECK (
        sample_size_hint IN ('unverified', 'low', 'medium', 'high')
    ),
    CONSTRAINT agent_reputation_dimension_scores_counts_valid CHECK (
        sample_size >= 0 AND passed_count >= 0 AND passed_count <= sample_size
        AND effective_sample >= 0 AND effective_passed >= 0
        AND effective_passed <= effective_sample + 1e-9
    ),
    CONSTRAINT agent_reputation_dimension_scores_rates_valid CHECK (
        raw_rate BETWEEN 0 AND 1
        AND lifetime_confidence BETWEEN 0 AND 1
        AND recent_confidence BETWEEN 0 AND 1
        AND score BETWEEN 0 AND 1
    ),
    -- 零观测必须同时是 sample_size=0 与 unverified，不能出现"零样本高分"。
    CONSTRAINT agent_reputation_dimension_scores_observed_consistent CHECK (
        observed = (sample_size > 0)
    ),
    CONSTRAINT agent_reputation_dimension_scores_evidence_object CHECK (
        jsonb_typeof(evidence) = 'object'
    )
);

-- 难度系数只能来自这张预先版本化的分类表，绝不接受 Agent 自报。
-- 区间 [0.75, 1.50] 是 doc §4.4 的硬性约束。
--
-- 主键含 algorithm_version：难度系数与维度权重同属评分参数，若全局唯一，
-- 调整一次系数就会静默改变所有历史投影的重算结果，等于原地改写历史。
CREATE TABLE task_difficulty_classes (
    algorithm_version TEXT NOT NULL,
    class TEXT NOT NULL,
    multiplier DOUBLE PRECISION NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (algorithm_version, class),
    CONSTRAINT task_difficulty_classes_params_fk
        FOREIGN KEY (algorithm_version)
        REFERENCES reputation_algorithm_params (algorithm_version)
        ON DELETE CASCADE,
    CONSTRAINT task_difficulty_classes_multiplier_range CHECK (
        multiplier >= 0.75 AND multiplier <= 1.50
    )
);

-- 首版参数。半衰期 180 天、recent 权重 0.70、Wilson 95% 单侧 z=1.96、
-- Jeffreys 先验 (0.5, 0.5)、最小可评分样本 5。
INSERT INTO reputation_algorithm_params (
    algorithm_version, dimension_weights, half_life_days,
    recent_weight_bps, wilson_z, prior_alpha, prior_beta, min_sample_for_score
) VALUES (
    '2026-09-09-v2',
    '{
        "correctness": 0.30,
        "reliability": 0.15,
        "reviewability": 0.10,
        "maintainability": 0.15,
        "security": 0.10,
        "collaboration": 0.10,
        "impact": 0.10
    }'::jsonb,
    180, 7000, 1.96, 0.5, 0.5, 5
);

-- 难度分级随参数版本一起 seed。新增算法版本时必须同时插入它自己的分级，
-- 否则 fact source 取不到系数（LEFT JOIN 落空回退 1.0）。
INSERT INTO task_difficulty_classes (algorithm_version, class, multiplier, description) VALUES
    ('2026-09-09-v2', 'trivial', 0.75, '单点修改，验收标准少且可自动判定'),
    ('2026-09-09-v2', 'standard', 1.00, '常规缺陷修复或小型特性'),
    ('2026-09-09-v2', 'substantial', 1.25, '跨模块改动或需要设计取舍'),
    ('2026-09-09-v2', 'complex', 1.50, '架构级改动、迁移或高风险区域');
