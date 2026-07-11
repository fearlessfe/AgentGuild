---
status: approved
change: agent-task-lifecycle
related-design: docs/superpowers/specs/2026-07-02-agent-task-lifecycle-design.md
---

# AgentGuild Production Compose 设计

> 2026-07-11 决策：首个可交付版本默认使用本地管理员登录，不要求外部
> OIDC。前端 Nginx 是唯一宿主机入口；后端与 PostgreSQL 不映射宿主机
> 端口。外部 OIDC 仍作为后续可选部署配置保留，不属于本次实现范围。

## 1. 目标与边界

提供一条可重复的命令：

```bash
docker compose up --build
```

该命令构建前端与后端镜像，执行数据库迁移，并启动 PostgreSQL、AgentGuild API 和只读 React Web。部署面向单机或由外部负载均衡承载的容器主机；TLS 在 Compose 外终止。

身份认证默认使用项目已有的本地管理员登录。操作者在 `.env` 中设置至少
12 字符的 `LOCAL_ADMIN_PASSWORD`，浏览器通过 `/oauth/local/login` 建立
HttpOnly session。仓库不提供可直接用于部署的默认密码，也不把密码、访问
令牌或固定 Bearer Token 编译进前端镜像。Langfuse 继续作为可选外部依赖，
不在本 Compose 内部署。

本设计不引入 Keycloak、Caddy、集群编排、自动证书、数据库备份或高可用 PostgreSQL。

## 2. 服务拓扑

```text
Browser
  │ HTTP（生产环境由外部设施升级为 HTTPS）
  ▼
frontend :8080 (Nginx)
  ├─ /, /assets/*       → React SPA
  ├─ /v1/*              → backend:8080
  └─ /mcp               → backend:8080
                           │
                           ▼
                     postgres:5432

migrate ──等待 postgres healthy──→ 顺序执行版本化 SQL ──成功──→ backend
```

Compose 定义四个服务：

| 服务 | 职责 | 对宿主机暴露 |
|---|---|---|
| `postgres` | PostgreSQL 18.4 与持久卷 | 默认不暴露 |
| `migrate` | 持有迁移锁、执行未应用 SQL、退出 | 不暴露 |
| `backend` | REST、MCP、reaper、outbox | 不暴露 |
| `frontend` | 静态资源、运行时配置、同源反向代理 | `${APP_PORT:-8080}` |

`backend` 必须等待 `migrate` 成功；`frontend` 必须等待 `backend` healthy。迁移或后端健康检查失败时，依赖服务不得伪装为正常启动。

## 3. 镜像构建

### 3.1 后端

`backend/Dockerfile` 使用多阶段构建：

1. Go 1.26.4 builder 下载依赖并构建静态 `agentguild-api`。
2. 最终镜像使用非 root 用户和最小运行时，包含 CA certificates，不包含 Go 工具链和源代码。
3. 容器监听 `:8080`，接收 SIGTERM 并使用现有 graceful shutdown。

镜像不得包含 `.env`、OIDC secret、Langfuse secret 或数据库密码。

### 3.2 前端

`frontend/Dockerfile` 使用 Node 构建 React 产物，再复制到 Nginx 非开发镜像。构建过程不得接收访问令牌或 OIDC client secret。

前端使用已有本地管理员登录界面，通过同源 `/oauth/local/login` 请求建立
session；后续 API 请求携带 session cookie。镜像构建过程不接收认证 secret。
外部 OIDC 运行时配置与 PKCE 支持不在本次 Compose 实现范围内。

## 4. 网络与路由

前端 Nginx 是唯一入口：

- SPA 路由使用 `try_files $uri $uri/ /index.html;`。
- `/v1/` 和 `/mcp` 同源代理到后端，保留 `Authorization`、请求 ID 和必要的转发头。
- 禁止为 API 响应启用静态缓存。
- 设置合理的请求体大小、连接/读取超时和基础安全响应头。
- `/healthz` 返回前端进程存活；后端提供独立、无需 OAuth 的 `/healthz`，只反映进程与数据库连接状态，不泄露配置。

外部代理负责 TLS、域名与可信转发头边界。Compose 内不开放 PostgreSQL 和后端宿主端口，降低误暴露风险。

## 5. 数据库迁移

`migrate` 使用 PostgreSQL 18.4 客户端和仓库中的 `backend/migrations/*.up.sql`：

1. 等待数据库健康。
2. 获取 PostgreSQL advisory lock，避免并发迁移。
3. 创建 `schema_migrations(version, applied_at)`。
4. 按文件名前缀升序执行尚未记录的 up migration；单个版本在事务内完成。
5. 成功后记录版本并释放锁；任一 SQL 失败则非零退出。

迁移不得依赖容器首次初始化目录，因为该机制无法处理已存在的数据卷。down migration 只保留给人工回滚，不在启动时自动执行。

## 6. 配置与 Secret

仓库提供 `.env.example`，真实 `.env` 继续被 Git 忽略。Compose 对以下值执行必填校验：

- `POSTGRES_PASSWORD`
- `CURSOR_SECRET`（至少 32 bytes）
- `SESSION_COOKIE_SECRET`（至少 32 bytes）
- `LOCAL_ADMIN_PASSWORD`（至少 12 字符）

默认租户、owner 与邮箱可通过 `LOCAL_ADMIN_TENANT_ID`、
`LOCAL_ADMIN_OWNER_ID`、`LOCAL_ADMIN_OWNER_EMAIL` 覆盖。Langfuse 启用时
额外要求其 base URL、public key 和 secret key。

Compose 变量仅用于本地/单机部署便利。生产系统可以用 Docker secrets 或部署平台 secret 注入覆盖 `.env`，但 Secret 不能成为 Docker build argument 或写入镜像层。

## 7. 启动、停止与故障语义

标准操作：

```bash
cp .env.example .env
docker compose up --build
docker compose down
```

数据卷默认保留；删除数据必须由操作者显式执行带 volume 的销毁命令。

故障处理：

- PostgreSQL 不健康：migrate/backend 不启动。
- 迁移失败：backend 不启动，日志指出失败版本。
- 本地管理员密码或 session secret 缺失/不合法：backend fail fast。
- Langfuse 禁用：不启动成本同步重试循环；核心任务生命周期继续运行。
- SIGTERM：后端先停止接收请求，取消 worker，等待 worker 退出，再关闭数据库池。

## 8. 测试与验收

### 静态验证

- `docker compose config` 成功解析。
- 后端、前端 Docker image 均能独立构建。
- Dockerfile 不包含 Secret build args，最终容器使用非 root 用户。

### 自动化测试

- 迁移服务首次启动成功，第二次启动不重复执行已应用版本。
- 缺少必填配置时 Compose/entrypoint 明确失败。
- Nginx SPA fallback、`/v1` 与 `/mcp` 代理规则测试通过。
- 本地管理员登录成功后可通过同源代理访问受保护 API；密码错误返回 401。
- 后端 `/healthz`、worker shutdown、Langfuse disabled 行为测试通过。

### 端到端验收

1. 从 `.env.example` 创建测试配置，设置本地管理员密码与随机 secret，执行 `docker compose up --build --wait`。
2. `postgres`、`backend`、`frontend` 均 healthy，`migrate` 以 0 退出。
3. 浏览器通过本地管理员密码登录后可以读取 `/tasks`，网络请求走同源 `/v1`。
4. 未登录请求返回 401，页面显示登录入口而不是嵌入 Token。
5. 执行 `docker compose down` 后再次启动，数据和 migration version 保留。

## 9. 计划修改文件

```text
docker-compose.yml
.env.example
.dockerignore
deploy/migrate.sh
backend/Dockerfile
backend/cmd/agentguild-api/main.go
backend/cmd/agentguild-api/main_test.go
frontend/Dockerfile
frontend/nginx.conf
frontend/src/features/auth/*
frontend/src/api/client.ts
frontend/src/**/*.test.tsx
```

实现应先补测试，再修改生产代码；Compose 验证必须实际构建并启动整套服务，不以单独的本地 Vite/Go 进程代替。
