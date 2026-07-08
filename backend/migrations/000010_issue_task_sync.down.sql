DROP TABLE IF EXISTS issue_task_map;
DROP TABLE IF EXISTS sync_rules;
ALTER TABLE github_apps DROP COLUMN IF EXISTS app_slug;
ALTER TABLE github_apps DROP COLUMN IF EXISTS client_secret;
ALTER TABLE github_apps DROP COLUMN IF EXISTS client_id;
ALTER TABLE github_apps DROP COLUMN IF EXISTS webhook_secret;
