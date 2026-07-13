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

GitHub App 是当前 tenant 下的多实例资源。管理员可以同时配置多个 App。这些 Web 接口只接受人类 session，不要使用 Agent access token。

以下读取和连接检测只要求当前 tenant 的已认证人类 session，普通 owner 与管理员均可调用：

- `GET /v1/github-apps`：返回当前 tenant 的全部 GitHub App public views；`data.items[].id` 是 tenant-scoped GitHub App ID。
- `GET /v1/github-apps/{id}`：返回当前 tenant 下指定 App 的 public view。
- `GET /v1/github-apps/{id}/repositories`：返回指定 App installation 可见的完整仓库列表。该平台接口不提供分页参数或 `next_cursor`；服务端会消费 GitHub 上游分页后在 `data.items` 中一次返回完整结果。
- `POST /v1/github-apps/{id}:test`：对指定 App 执行脱敏的连接检测；该检测不修改配置。

以下操作会修改配置或仓库治理清单，handler 要求当前 tenant 的管理员 session：

- `DELETE /v1/github-apps/{id}`：删除未被已接入仓库引用的 App。
- `POST /v1/repositories/github-app`：把已选 App 可见的仓库加入平台治理清单。请求体必须显式携带 `github_app_id`，不应依赖默认 App。
- `POST /v1/repositories/public` 与 `DELETE /v1/repositories/{id}`：接入公开仓库或移除已接入仓库。
- `DELETE /v1/github-apps/{id}`、上述两个仓库 POST 以及仓库 DELETE 都必须携带 `Idempotency-Key`。同一逻辑操作在网络中断或响应未知时必须复用原 key；操作成功或收到确定的 HTTP 响应后，新操作使用新 key。服务端会原样重放已完成响应，并拒绝同 key 的不同请求。
- `POST /v1/github-app` 与 `DELETE /v1/github-app`：写入或删除兼容的默认 App 配置。`GET /v1/github-app` 和非修改性的 `POST /v1/github-app:test` 只要求已认证人类 session。
- `/oauth/github/app/manifest`、`/oauth/github/app/install`、`/oauth/github/app/callback` 与 `/oauth/github/app/installed`：创建、安装或更新 App 配置的 onboarding 流程，均要求管理员 session。

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
