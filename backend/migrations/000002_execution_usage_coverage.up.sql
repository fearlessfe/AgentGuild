-- 将 execution_usage.coverage 的合法值从 'complete' 改为 'full'，与领域枚举保持一致。
ALTER TABLE execution_usage DROP CONSTRAINT execution_usage_coverage_valid;
ALTER TABLE execution_usage ADD CONSTRAINT execution_usage_coverage_valid
    CHECK (coverage IN ('full', 'partial', 'unavailable'));
