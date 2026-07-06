# Brainstorm Summary

- Change: frontend-capability-gaps
- Date: 2026-07-05

## 确认的技术方案

### 登录入口
- 新增 `frontend/src/features/auth/LoginPage.tsx`：独立登录页，居中展示 AgentGuild 品牌、说明文字和"使用企业 OIDC 登录"按钮。
- 按钮点击后调用 `window.location.href = "/oauth/oidc/login"`，由后端完成 OIDC 流程并在 callback 设置 session cookie 后跳转回首页 `/agents`。
- 在 `AppShell.tsx` 路由表末尾加入 `/login` 路由；非登录态访问受保护页面时由后端返回 401，前端在 `apiRequest` 捕获 401 后统一跳转 `/login`。
- Demo 模式下 `/login` 仍可用，但点击后跳转回首页并模拟已登录（便于 Playwright e2e 与离线开发）。

### 基准集创建表单
- 替换 `EvaluationList.tsx` 中的 `BenchmarkSetForm` 占位实现。
- 表单字段：
  - `name`（必填，text input）
  - `description`（可选，textarea）
  - `tasks`（必填，textarea，按逗号/换行/空格分隔解析为 `string[]`）
  - `is_active`（checkbox，默认 true）
- 提交时调用 `evaluations.api.ts` 的 `createBenchmarkSet`，成功后通过 `onCreated` 回调通知父组件刷新列表。
- 表单使用受控组件 + 本地校验（空 name/tasks 时禁用提交并提示）。

### 类型对齐
- `BenchmarkSetView` 保持与后端 `BenchmarkSetSummary` 一致：`id`、`tenant_id`、`version_number`、`name`、`description`、`is_active`、`created_by`、`created_at`。
- `EvaluationRunView` 与 `backend-response-format-alignment` 产出的 `EvaluationRunDetail` 一致：`id`、`tenant_id`、`agent_version_id`、`benchmark_set_id`、`status`、`environment_digest`、`scoring_rule_version`、`threshold_results`、`summary`、`started_at`、`completed_at`。
- `EvaluationDetail` 与 `EvaluationList` 组件增加对缺失 `summary` / `threshold_results` 的边界处理（展示"结果聚合中"或空状态），避免 `undefined` 运行时错误。

### API / Demo 模式
- 在 `client.ts` 的 `demo()` 中补充：
  - `GET /v1/benchmarks`：返回示例基准集列表。
  - `POST /v1/benchmarks`：解析 body，追加新基准集并返回 `{ benchmark_set_id, version_number }`。
  - `GET /v1/evaluations`：返回示例评测运行列表。
  - `GET /v1/evaluations/:id`：返回包含完整 `threshold_results` 与 `summary` 的详情。
- 新增 `unwrapEvaluationResponse<T>` 兼容辅助函数：后端 evaluation 接口目前返回裸数组/对象，而前端 `apiRequest` 期望 `Envelope<T>`。在 `evaluations.api.ts` 中先尝试按 Envelope 解析，否则按原始 payload 包装，保证与当前后端及对齐后的后端都能工作。

### 测试
- 新增 `LoginPage.test.tsx`：渲染按钮、点击跳转。
- 扩展 `evaluations.test.tsx`：测试 `BenchmarkSetForm` 输入、提交、校验，以及 `EvaluationDetail` 对缺失 summary 的渲染。
- 运行 `npm test -- --run` 与 `npm run build`。

## 关键取舍与风险

| 取舍 | 方案 | 风险 | 缓解 |
|---|---|---|---|
| OIDC 登录页是否内嵌后端跳转 | 前端只做跳转按钮，实际认证由后端 `/oauth/oidc/login` 处理 | 本地无 OIDC 配置时无法完整测试 | Demo 模式保留离线可用 |
| 后端 evaluation 接口响应格式不一致 | 前端 API 层做 Envelope/裸响应兼容 | 过渡期代码冗余 | 后端对齐完成后可移除兼容层；本 change 不修改后端 |
| `tasks` 输入格式 | 前端文本框解析为 `string[]`，与后端 `[]string` 保持一致 | 用户输入格式不统一 | 支持逗号、换行、空格多种分隔符，并做 trim/去空 |
| 是否新增独立 `auth` feature 目录 | 是，放置 `LoginPage.tsx` 与测试 | 目录浅但可扩展 | 未来如需登出/用户信息可继续扩展 |

## 测试策略

- 前端单元测试覆盖新增组件与表单交互。
- Demo 模式下手动验证登录跳转、基准集创建、评测列表与详情渲染。
- 与 `backend-response-format-alignment` 集成后验证真实后端响应。

## Spec Patch

- 建议在 `openspec/specs/evaluation/spec.md` 的"基准集与评测运行 REST 视图字段使用 snake_case"要求中补充：响应应使用 `Envelope<T>` 结构（`data` + `meta`），与控制台前端 `apiRequest` 约定保持一致。
- 该补充作为对 `backend-response-format-alignment` change 的协调建议，不在本 change 中实现后端修改。
