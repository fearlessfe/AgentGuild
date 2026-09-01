# GitHub Issue 同步功能 - 本地开发与冒烟测试指南

> 本文档对应 Comet change `github-issue-task-sync` Tasks 7.2 / 7.3，记录本地启动流程与冒烟验证路径。

---

## 1. 本地环境启动（Task 7.2）

### 1.1 启动 Postgres

```bash
make db-up
```

### 1.2 应用数据库迁移

```bash
# 按序执行所有迁移脚本（必须包含当前最新迁移）
for f in backend/migrations/0000*_*.up.sql; do
  psql "$DATABASE_URL" -f "$f"
done
```

> 若需单步执行，可逐一运行 `backend/migrations/` 目录下的 `.up.sql` 文件。

### 1.3 加载环境变量

```bash
source .local/env.sh
```

`.local/env.sh` 中需包含以下关键变量（至少）：

| 变量 | 说明 |
|------|------|
| `DATABASE_URL` | Postgres 连接串 |
| `LOCAL_ADMIN_PASSWORD` | 本地登录密码 |
| `GITHUB_APP_PUBLIC_BASE_URL` | `WEB_ENABLED=true` 时必填；AgentGuild 的公网 HTTP(S) origin，不包含路径、query 或 fragment |

### 1.4 启动后端

```bash
cd backend && go run ./cmd/agentguild-api
```

后端默认监听 `:8080`。

### 1.5 启动前端

```bash
cd frontend && npm run dev
```

Vite 开发服务器启动后，`/api` 路径自动代理到 `http://localhost:8080`。

### 1.6 本地登录

```bash
curl -s -c cookies.txt -X POST http://localhost:8080/oauth/local/login \
  -H 'Content-Type: application/json' \
  -d '{"password": "<LOCAL_ADMIN_PASSWORD>"}'
```

成功后响应会设置 `agentguild_session` Cookie，后续请求携带该 Cookie 即可通过鉴权。

---

## 2. GitHub App 接入 - 两条路径（Task 7.3）

### Path A：隧道方案（推荐，可用公网时）

1. 启动隧道，将本地 `:8080` 暴露到公网：

   ```bash
   # 使用 cloudflared
   cloudflared tunnel --url http://localhost:8080

   # 或使用 ngrok
   ngrok http 8080
   ```

2. 将隧道输出的公网 URL（如 `https://xxxx.trycloudflare.com`）写入环境变量并重启后端：

   ```bash
   # 只填 origin，不要附加 /oauth/... 路径、query 或 fragment
   export GITHUB_APP_PUBLIC_BASE_URL=https://xxxx.trycloudflare.com
   cd backend && go run ./cmd/agentguild-api
   ```

3. 在前端页面 **Settings → Git Integration** 点击「Connect GitHub」，系统将自动走 GitHub App Manifest 流完成 App 创建与安装授权。

### Path B：降级方案（无公网时手动配置）

当无法获得公网地址时，可通过 API 直接写入 GitHub App 配置，效果等同于 Manifest 流完成后的状态：

```bash
curl -s -b cookies.txt -X POST http://localhost:8080/v1/github-app \
  -H 'Content-Type: application/json' \
  -d '{
    "app_id": <YOUR_APP_ID>,
    "installation_id": <YOUR_INSTALLATION_ID>,
    "private_key": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
  }'
```

> `app_id` 和 `installation_id` 可在 GitHub → Settings → Developer settings → GitHub Apps 中获取；`private_key` 为 PEM 格式私钥。

---

## 3. 冒烟测试步骤（Task 7.3）

### 3.1 创建同步规则

```bash
curl -s -b cookies.txt -X POST http://localhost:8080/v1/sync-rules \
  -H 'Content-Type: application/json' \
  -d '{
    "repo": "org/repo-name",
    "issue_state": "all",
    "include_labels": ["agent-ready"],
    "task_type": "code",
    "source_auth": "public",
    "dedupe_strategy": "update"
  }'
# 记录返回的 sync rule id，后续使用
```

### 3.2 手动触发一次同步

```bash
RULE_ID=<sync-rule-id>
curl -s -b cookies.txt -X POST "http://localhost:8080/v1/sync-rules/${RULE_ID}:run"
```

### 3.3 验证任务创建

- 打开前端 **Task Center**，确认出现从 GitHub Issue 生成的任务。
- 每个任务应带有来源标签，格式类似 `Issue #N @ org/repo-name`。
- 规则使用 `issue_state=all` 是为了让后续轮询能够观察到 Issue 关闭并处理已存在映射；必须配合专用标签，避免首次同步拉取整个仓库的历史 Issue。

### 3.4 验证取消逻辑

1. 在 GitHub 上关闭对应 Issue。
2. 再次触发同步：

   ```bash
   curl -s -b cookies.txt -X POST "http://localhost:8080/v1/sync-rules/${RULE_ID}:run"
   ```

3. 检查 Task Center：**未认领（unclaimed）** 的对应任务状态应变为 `cancelled`；已认领任务不受影响。

### 3.5 验证去重逻辑

再次触发同步（Issue 仍为关闭状态或重新打开同一 Issue）：

```bash
curl -s -b cookies.txt -X POST "http://localhost:8080/v1/sync-rules/${RULE_ID}:run"
```

Task Center 中**不应**出现重复任务，确认去重机制正常工作。

---

## 4. 常见问题

| 现象 | 排查方向 |
|------|---------|
| 后端启动报 `DATABASE_URL not set` | 检查是否执行了 `source .local/env.sh` |
| GitHub App Manifest 回调失败 | 确认 `GITHUB_APP_PUBLIC_BASE_URL` 已设置且隧道仍在运行 |
| 同步后任务未出现 | 检查 Issue 是否带有 `agent-ready` 标签；查看后端日志确认 sync job 是否报错 |
| 关闭 Issue 后任务未取消 | 确认规则使用 `issue_state=all` 且 Issue 仍带包含标签；已认领任务会计入 `flagged`，不会被强制取消 |
| 重复任务出现 | 确认当前全部迁移已应用，尤其是 `issue_task_map` 的唯一键 |
