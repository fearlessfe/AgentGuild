## Context

控制台目前把 OIDC 登录交给外部或 Vite 代理，前端路由里没有显式登录页。人类用户首次打开控制台时缺少跳转引导。评测模块的 `BenchmarkSetForm` 只是占位，无法真正创建基准集。`EvaluationDetail` 依赖 `threshold_results` 和 `summary` 字段，但后端 summary 接口不提供。

## Goals / Non-Goals

**Goals:**
- 提供可跳转的 OIDC 登录入口。
- 提供可用的基准集创建表单。
- 与 `backend-response-format-alignment` change 协同后，评测详情和基准集详情不再出现 undefined 错误。

**Non-Goals:**
- 不实现完整的用户权限管理 UI。
- 不替代后端 OIDC 服务端点。
- 不修改后端 evaluation 领域模型（由另一个 change 负责）。

## Decisions

1. **登录入口**
   - 在 `AppShell` 外新增独立 `/login` 路由，点击后跳转到后端 `/oauth/oidc/login`。
   - 未登录时由后端返回 401，前端统一跳转到 `/login`。

2. **基准集表单**
   - 使用受控组件实现 `name`、`description`、`tasks`（逗号/换行分隔的 task refs）、`is_active` 字段。
   - 提交到 `POST /v1/benchmarks`，成功后刷新列表。

3. **类型对齐依赖**
   - 本 change 假设 `backend-response-format-alignment` 会提供 `EvaluationRunDetail` 和 `BenchmarkSetDetail` 的完整字段；本 change 只调整前端类型以匹配最终约定。

## Risks / Trade-offs

- [Risk] OIDC 登录跳转在本地开发时需要后端已配置 OIDC provider，否则无法测试。
  - Mitigation: 真实模式需要后端 OIDC；Demo 模式保留离线可用。
- [Risk] `BenchmarkSetForm` 的 `tasks` 输入格式需要与后端 `BenchmarkTask` 对齐（目前后端接收 `[]string` 并自动设置 ordering）。
  - Mitigation: 前端先按简单字符串列表提交，与后端现有路由保持一致。

## Open Questions

- 登录成功后前端如何感知？后端 callback 设置 cookie 后跳转回前端首页即可。
