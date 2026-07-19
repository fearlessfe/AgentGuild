-- 版本晋级审批记录：promoted_by 记录执行晋级的审批人。
-- promoted_at 已由 000008 引入（可空 TIMESTAMPTZ），此处仅兜底保证存在。
ALTER TABLE IF EXISTS agent_versions
    ADD COLUMN IF NOT EXISTS promoted_by TEXT,
    ADD COLUMN IF NOT EXISTS promoted_at TIMESTAMPTZ;
