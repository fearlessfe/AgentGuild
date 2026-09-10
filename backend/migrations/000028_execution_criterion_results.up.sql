-- 验收标准此前只有规格（public_task_projections.acceptance_criteria），没有结果。
-- 本迁移补上逐条 criterion 的验证事实：append-only、按验证来源幂等，
-- 供奖励按 criterion 分段释放与声望 correctness 维度重放。

CREATE TABLE execution_criterion_results (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_tenant_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    execution_id TEXT NOT NULL,
    criterion_id TEXT NOT NULL,
    critical BOOLEAN NOT NULL,
    verifier_kind TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    -- 验证来源。同一 source 对同一 criterion 只能落一条，重复投递为 no-op。
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    verified_by TEXT NOT NULL,
    verifier_version TEXT NOT NULL DEFAULT '',
    evidence_uri TEXT NOT NULL DEFAULT '',
    evidence_hash TEXT NOT NULL DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT execution_criterion_results_execution_fk
        FOREIGN KEY (resource_tenant_id, execution_id, task_id)
        REFERENCES executions (tenant_id, id, task_id),
    CONSTRAINT execution_criterion_results_verifier_kind_valid CHECK (
        verifier_kind IN ('command', 'ci', 'manual')
    ),
    CONSTRAINT execution_criterion_results_source_kind_valid CHECK (
        source_kind IN ('validation_job', 'review', 'manual_override')
    ),
    CONSTRAINT execution_criterion_results_nonempty CHECK (
        criterion_id <> '' AND source_id <> '' AND verified_by <> ''
    ),
    CONSTRAINT execution_criterion_results_evidence_hash_valid CHECK (
        evidence_hash = '' OR evidence_hash ~ '^[0-9a-f]{64}$'
    )
);

-- 幂等键：同一验证来源对同一 criterion 的重复投递被数据库拒绝。
CREATE UNIQUE INDEX execution_criterion_results_source_key
    ON execution_criterion_results (
        resource_tenant_id, execution_id, criterion_id, source_kind, source_id
    );

CREATE INDEX execution_criterion_results_execution_order
    ON execution_criterion_results (
        resource_tenant_id, execution_id, criterion_id, observed_at DESC, id DESC
    );

-- 每个 criterion 的最新态。人工结论晚于自动结论时以人工为准，
-- 因此只按 observed_at 排序，不给 source_kind 额外优先级。
CREATE VIEW execution_criterion_latest AS
SELECT DISTINCT ON (resource_tenant_id, execution_id, criterion_id)
    id,
    resource_tenant_id,
    task_id,
    execution_id,
    criterion_id,
    critical,
    verifier_kind,
    passed,
    source_kind,
    source_id,
    verified_by,
    verifier_version,
    evidence_uri,
    evidence_hash,
    observed_at,
    recorded_at
FROM execution_criterion_results
ORDER BY resource_tenant_id, execution_id, criterion_id, observed_at DESC, id DESC;

CREATE FUNCTION reject_execution_criterion_result_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'execution_criterion_results are append-only'
        USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER execution_criterion_results_reject_update
    BEFORE UPDATE ON execution_criterion_results
    FOR EACH ROW EXECUTE FUNCTION reject_execution_criterion_result_mutation();

CREATE TRIGGER execution_criterion_results_reject_delete
    BEFORE DELETE ON execution_criterion_results
    FOR EACH ROW EXECUTE FUNCTION reject_execution_criterion_result_mutation();

-- 难度分级只能由 analyzer 写入，不接受 Agent 自报（doc §4.4）。
-- spec_hash 让 RewardDecision 能引用一个稳定的任务规格摘要。
ALTER TABLE public_task_projections
    ADD COLUMN difficulty_class TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN spec_hash TEXT NOT NULL DEFAULT '';

ALTER TABLE public_task_projections
    ADD CONSTRAINT public_task_projections_difficulty_class_valid CHECK (
        difficulty_class IN ('trivial', 'standard', 'substantial', 'complex')
    ),
    ADD CONSTRAINT public_task_projections_spec_hash_valid CHECK (
        spec_hash = '' OR spec_hash ~ '^[0-9a-f]{64}$'
    );
