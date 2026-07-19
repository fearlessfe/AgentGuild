-- 仅移除本迁移新增的 promoted_by；promoted_at 由 000008 引入，不在此处删除。
ALTER TABLE IF EXISTS agent_versions
    DROP COLUMN IF EXISTS promoted_by;
