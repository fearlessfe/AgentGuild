# Bifrost Issue 接入 MVP

本文档描述将公开仓库 `maximhq/bifrost` 接入 AgentGuild 的最小可用流程。这个版本只负责把带指定标签的 GitHub Issue 转换为 AgentGuild Task，不包含自动分析、自动创建 Pull Request 或外部 Agent 的跨租户 Git 写权限。

## MVP 范围

```text
Bifrost Issue
  -> public repository onboarding
  -> sync rule
  -> manual/periodic sync
  -> AgentGuild Task
```

建议先在 Bifrost 创建专用标签 `agentguild-ready`，只给明确希望交给 Agent 处理的 Issue 加这个标签。不要用空的 `include_labels` 同步整个仓库。

## 启动

先按 [local-dev-github-issue-sync.md](local-dev-github-issue-sync.md) 启动 PostgreSQL、后端和前端。最小本地配置需要：

- `DATABASE_URL`
- 至少 32 字节的 `CURSOR_SECRET` 和 `SESSION_COOKIE_SECRET`
- 至少 12 个字符的 `LOCAL_ADMIN_PASSWORD`
- Web 模式下合法的 `GITHUB_APP_PUBLIC_BASE_URL`，例如 `http://localhost:8080`

公开 Issue 读取不需要 GitHub App 私钥。`GITHUB_APP_PUBLIC_BASE_URL` 仍是 Web/MCP 启动时的必填配置，因为它同时服务于 GitHub App Manifest 路径。

## 接入仓库和规则

登录后，以管理员 session 执行：

```bash
BASE_URL=http://localhost:8080

curl -sS -b cookies.txt -X POST "$BASE_URL/v1/repositories/public" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: bifrost-repository-add-1' \
  -d '{"repo":"maximhq/bifrost"}'
```

创建同步规则：

```bash
curl -sS -b cookies.txt -X POST "$BASE_URL/v1/sync-rules" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: bifrost-sync-rule-create-1' \
  -d '{
    "repo":"maximhq/bifrost",
    "include_labels":["agentguild-ready"],
    "exclude_labels":["wontfix"],
    "issue_state":"all",
    "task_type":"code",
    "default_priority":"normal",
    "dedupe_strategy":"update",
    "source_auth":"public",
    "enabled":true
  }'
```

记录响应里的规则 ID，然后手动运行：

```bash
RULE_ID=<sync-rule-id>
curl -sS -b cookies.txt -X POST "$BASE_URL/v1/sync-rules/$RULE_ID:run" \
  -H 'Idempotency-Key: bifrost-sync-run-1'
```

`issue_state=all` 用于让同步器观察到已映射 Issue 的关闭状态。由于它会返回开放和关闭 Issue，必须使用专用标签控制范围。公开 GitHub API 未认证时有请求额度限制，MVP 阶段建议以手动触发为主，或把 `SYNC_WORKER_INTERVAL` 调大到 10 分钟以上。

## 验收清单

- 带 `agentguild-ready` 标签的 Bifrost Issue 生成一个 Task。
- Task 详情显示来源仓库、Issue 编号和 GitHub 链接。
- 再次运行同步不会生成重复 Task。
- 修改 Issue 标题或正文时，未领取 Task 会更新。
- 关闭未领取 Issue 时，Task 会变为 `cancelled`。
- 关闭已领取 Issue 时，Task 不会被强制取消，同步结果中的 `flagged` 会增加。

## 当前限制

公开仓库接入目前是 Issue 读取路径。公开仓库的 Git credential、fork/PR 目标和公共任务跨租户授权还没有接入；要让 Agent 修改代码，应先把 Bifrost 的 fork 通过 GitHub App 接入，再走现有 Claim → credential → branch → Submission 流程。
