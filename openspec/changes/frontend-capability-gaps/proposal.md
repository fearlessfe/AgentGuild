## Why

人类控制台目前缺少 OIDC 登录入口和创建评测基准集的表单，导致用户无法完成登录闭环，也无法在 UI 中新建基准集。此外，部分已有组件（如 `EvaluationDetail`、`BenchmarkSetView`）的类型与后端真实响应结构不一致，运行时会出现 undefined 错误。补齐这些 UI 能力和类型对齐是让控制台真正可用的一环。

## What Changes

- 在前端新增 OIDC 登录入口（路由 `/login` 或跳转 `/oauth/oidc/login`）。
- 新增或完善 `BenchmarkSetForm`，使其可调用 `POST /v1/benchmarks` 创建基准集。
- 对齐 `frontend/src/features/evaluations/evaluations.types.ts` 与后端实际返回：`BenchmarkSetView` 增加/调整字段，`EvaluationRunView` 与 `backend-response-format-alignment` change 协同。
- 调整 `frontend/src/api/client.ts` 中的 demo 数据以覆盖新接口（或新增真实模式处理）。
- 补充前端单元测试。

## Capabilities

### New Capabilities

- `console-login`：人类控制台 OIDC 登录入口。
- `benchmark-set-management`：在控制台创建和查看评测基准集。

### Modified Capabilities

- `evaluation`：基准集和评测运行接口的响应结构需与前端类型保持一致（与 `backend-response-format-alignment` 协同）。

## Impact

- `frontend/src/app/AppShell.tsx` 路由。
- `frontend/src/features/evaluations/*`。
- 可能新增 `frontend/src/features/auth/*`。
- `frontend/src/api/client.ts`。
- Playwright e2e 配置和测试可能需调整登录流程。
