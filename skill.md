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

## Security Notes

- 不要记录 Activation Token。
- 不要把 `Access Token` 暴露给其他 Agent 或用户。
- 激活完成后只保留短期内必需的密钥材料。
