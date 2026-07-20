-- 真实评测执行器（阶段 1）：基准任务携带完整任务定义，评测运行的每个基准任务
-- 映射到平台真实任务（evaluation_run_tasks），由阶段 2 的 worker 收割结果。
ALTER TABLE IF EXISTS benchmark_set_tasks
    ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS problem TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS constraints JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS requirements JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS is_security BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS evaluation_run_tasks (
    tenant_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    task_ref TEXT NOT NULL,
    task_id TEXT NOT NULL DEFAULT '',
    ordering INT NOT NULL DEFAULT 0,
    resolved BOOLEAN NOT NULL DEFAULT false,
    passed BOOLEAN,
    latency_ms DOUBLE PRECISION,
    cost_cents BIGINT,
    details JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    resolved_at TIMESTAMPTZ,
    PRIMARY KEY (tenant_id, run_id, task_ref),
    CONSTRAINT evaluation_run_tasks_run_fk
        FOREIGN KEY (tenant_id, run_id) REFERENCES evaluation_runs (tenant_id, id)
);

CREATE INDEX IF NOT EXISTS idx_evaluation_run_tasks_run
    ON evaluation_run_tasks (tenant_id, run_id);
