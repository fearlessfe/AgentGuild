ALTER TABLE execution_usage DROP CONSTRAINT execution_usage_coverage_valid;
ALTER TABLE execution_usage ADD CONSTRAINT execution_usage_coverage_valid
    CHECK (coverage IN ('complete', 'partial', 'unavailable'));
