## ADDED Requirements

### Requirement: 本地开发一键启动
系统 SHALL 提供可复现的本地开发启动流程，一次性拉起 PostgreSQL、后端 API 与前端开发服务器，并加载本地开发所需的环境变量。

#### Scenario: 开发者从零启动本地全栈
- **WHEN** 开发者在干净环境执行文档化的本地启动流程（含 `docker compose` 启动 Postgres、加载 `.local/env.sh`、启动 backend 与 frontend）
- **THEN** 后端在 `:8080` 监听、前端在 Vite 开发端口可访问，且 `/api` 代理可将前端请求转发到后端

### Requirement: 本地 GitHub App 配置
系统 SHALL 支持在本地开发环境配置真实的 per-tenant GitHub App 安装，使 Issue→Task 同步可对接真实 GitHub。

#### Scenario: 本地配置 GitHub App 后可列仓库
- **GIVEN** 开发者已在本地为租户配置有效 GitHub App（App ID、安装 ID、私钥）
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回该安装可访问的仓库列表

### Requirement: 前端默认连接真实后端并保留 demo 降级
系统 SHALL 使前端在未显式开启 demo 模式时默认连接真实后端；当 `VITE_DEMO_MODE=true` 时前端 SHALL 使用内置 fixtures 离线运行。

#### Scenario: 默认走真实后端
- **WHEN** 前端在未设置 `VITE_DEMO_MODE`（或其值不为 `true`）的情况下运行并发起 API 请求
- **THEN** 请求经 `/api` 发往真实后端，而非内置 demo fixtures

#### Scenario: 显式开启 demo 降级
- **WHEN** 前端以 `VITE_DEMO_MODE=true` 运行
- **THEN** API 请求由内置 fixtures 响应，无需后端即可展示核心页面
