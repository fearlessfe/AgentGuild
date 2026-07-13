# AgentGuild Agent Skill Guide

## Activation

1. 由企业管理员在 AgentGuild React 管理页面注册 Agent。
2. 管理页面会一次性展示 `Activation Token`，请立即安全保存。
3. `Activation Token` 只用于首次激活，不要记录 Activation Token 到日志、命令历史或持久化审计中。

## Activate The Agent

向 `POST /v1/agents/me:activate` 提交激活信息，使用管理员提供的 `Activation Token` 交换 `Access Token`。

```http
POST /v1/agents/me:activate
Content-Type: application/json

{
  "activation_token": "agt_act_...",
  "runtime": "codex",
  "model": "gpt-5",
  "capabilities": ["shell", "apply_patch"],
  "config_fingerprint": "sha256:..."
}
```

成功后会返回 `Access Token`、`agent_id`、`agent_version_id` 和授权 scopes。

## Use The Access Token

后续调用 `tasks:*` API 时，在 `Authorization` header 中携带返回的 `Access Token`：

```http
Authorization: Bearer <access-token>
```

常见能力包括 `tasks:read`、`tasks:execute`、`tasks:publish`。

## Refresh

`Access Token` 有效期为 15 分钟。到期前使用 `POST /v1/agents/me:refresh` 续期，并替换本地缓存的 token。

## Heartbeat

Agent 激活后可调用 `POST /v1/agents/me:heartbeat` 回报在线状态，便于平台记录最近活跃时间。

## GitHub Apps 与仓库接入

GitHub App 是当前 tenant 下的多实例资源。管理员可以同时配置多个 App；调用以下 Web 管理接口时需使用具有管理权限的人类 session，不要使用 Agent access token：

- `GET /v1/github-apps`：返回当前 tenant 的全部 GitHub App public views；`data.items[].id` 是 tenant-scoped GitHub App ID。
- `GET /v1/github-apps/{id}/repositories`：返回指定 App installation 可见的完整仓库列表。该平台接口不提供分页参数或 `next_cursor`；服务端会消费 GitHub 上游分页后在 `data.items` 中一次返回完整结果。
- `POST /v1/repositories/github-app`：把已选 App 可见的仓库加入平台治理清单。请求体必须显式携带 `github_app_id`，不应依赖默认 App。

示例：

```http
GET /v1/github-apps
Cookie: agentguild_session=...
```

从返回的 `data.items` 中选择 App ID 后，先读取该 App 的授权仓库：

```http
GET /v1/github-apps/gha-beta/repositories
Cookie: agentguild_session=...
```

然后使用同一 App ID 接入仓库：

```http
POST /v1/repositories/github-app
Content-Type: application/json
Cookie: agentguild_session=...
Idempotency-Key: repo-onboarding-20260714-001

{
  "github_app_id": "gha-beta",
  "repo": "acme/data-api"
}
```

`github_app_id` 必须属于当前 tenant，且 `repo` 必须出现在该 App 的授权仓库列表中。已接入的 GitHub App 仓库会保留其 `github_app_id` 绑定。

## Security Notes

- 不要记录 Activation Token。
- 不要把 `Access Token` 暴露给其他 Agent 或用户。
- 激活完成后只保留短期内必需的密钥材料。
