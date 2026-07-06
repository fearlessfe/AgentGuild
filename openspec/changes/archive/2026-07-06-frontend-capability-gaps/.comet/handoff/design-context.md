# Comet Design Handoff

- Change: frontend-capability-gaps
- Phase: design
- Mode: compact
- Context hash: 1b9dd3b54acfd88c372e26fea0c83ce8715571c0f8c91fb8e24e0195ffc147c3

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/frontend-capability-gaps/proposal.md

- Source: openspec/changes/frontend-capability-gaps/proposal.md
- Lines: 1-30
- SHA256: 7e292074c346e86d1db31935677230a9bb42043561835103ab51f9c63e19c1b1

```md
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
```

## openspec/changes/frontend-capability-gaps/design.md

- Source: openspec/changes/frontend-capability-gaps/design.md
- Lines: 1-39
- SHA256: 4b9b9aa245fa7118fd14dd989ab3b1b58e12550ddbe3f26736eeedcd6ba5806a

```md
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
```

## openspec/changes/frontend-capability-gaps/tasks.md

- Source: openspec/changes/frontend-capability-gaps/tasks.md
- Lines: 1-28
- SHA256: 3a47d88fdf6b5f146ceec38022938a037f60689f015423624e3379db2d476aa1

```md
## 1. 登录入口

- [ ] 1.1 新增 `frontend/src/features/auth/LoginPage.tsx`（或类似）提供 OIDC 登录跳转
- [ ] 1.2 在 `AppShell.tsx` 路由中加入 `/login`
- [ ] 1.3 处理 401 统一跳转登录页（可选：在 `apiRequest` 中捕获 401 后跳转）

## 2. 基准集创建

- [ ] 2.1 实现可提交基准集创建表单的 `BenchmarkSetForm`
- [ ] 2.2 在 `frontend/src/features/evaluations/evaluations.api.ts` 中确认 `createBenchmarkSet` 调用正确
- [ ] 2.3 在 `frontend/src/api/client.ts` demo 模式中补充 `/v1/benchmarks` POST 处理

## 3. 类型对齐

- [ ] 3.1 调整 `frontend/src/features/evaluations/evaluations.types.ts` 中 `BenchmarkSetView` 和 `EvaluationRunView` 以匹配后端最终 DTO
- [ ] 3.2 更新 `EvaluationList` / `EvaluationDetail` 组件，处理字段缺失的边界情况

## 4. 测试与验证

- [ ] 4.1 为登录入口添加前端单元测试
- [ ] 4.2 为基准集表单添加前端单元测试
- [ ] 4.3 运行 `npm test -- --run`
- [ ] 4.4 运行 `npm run build`

## 5. 文档与收尾

- [ ] 5.1 更新 change tasks.md
- [ ] 5.2 运行 Comet open 阶段守卫
```

