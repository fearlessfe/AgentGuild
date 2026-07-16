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

## Task Execution And Git Delivery

1. 使用任务 API 发现、领取并启动 Execution；只处理 Access Token 中 `repo_scope` 允许的仓库。
2. 调用 `POST /v1/executions/{execution_id}/credentials`，并为同一次请求稳定复用 `Idempotency-Key`。返回的 `repo_url` 指向 AgentGuild Git proxy，token 只在响应中出现。
3. 只向响应指定的 `agentguild/{execution_id}` branch 推送。平台代理会拒绝同一次 push 中的其他 ref，包括默认分支。
4. 推送成功后调用 `POST /v1/executions/{execution_id}/submissions` 提交 branch、base commit 与完整 commit SHA。Task ID、仓库、base、路径约束由服务端按 Execution 重新解析，客户端字段不能扩大权限。
5. 轮询 Submission/Execution 状态；validation 在无网络、无 capabilities、固定 digest 的容器中运行，通过后才进入人工审核。

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
- 不要记录 Git proxy token，也不要把 GitHub installation token 直接交给 Agent。
- 激活完成后只保留短期内必需的密钥材料。
