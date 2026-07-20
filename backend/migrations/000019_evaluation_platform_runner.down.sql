-- 回滚 000019：删除评测运行任务映射表与基准任务定义字段。
DROP TABLE IF EXISTS evaluation_run_tasks;

ALTER TABLE IF EXISTS benchmark_set_tasks
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS problem,
    DROP COLUMN IF EXISTS constraints,
    DROP COLUMN IF EXISTS requirements,
    DROP COLUMN IF EXISTS is_security;
