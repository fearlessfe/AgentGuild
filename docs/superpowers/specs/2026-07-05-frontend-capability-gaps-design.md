---
comet_change: frontend-capability-gaps
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-06-frontend-capability-gaps
status: final
---

# Frontend Capability Gaps — Design Doc

## 背景

AgentGuild 人类控制台目前缺少 OIDC 登录入口和创建评测基准集的表单。关闭 `VITE_DEMO_MODE` 后：

- 未登录用户没有引导跳转到 OIDC 登录。
- `BenchmarkSetForm` 只是占位，无法真正调用 `POST /v1/benchmarks`。
- `EvaluationDetail` 直接访问 `threshold_results` 与 `summary`，但后端当前 `EvaluationRunSummary` 不提供这些字段，运行时会出现 `undefined` 错误。

本 change 补齐上述 UI 能力，并与 `backend-response-format-alignment` change 协同对齐评测相关类型。

## 目标

- 提供可跳转的 OIDC 登录入口（`/login`）。
- 提供可用的基准集创建表单。
- 对齐 `frontend/src/features/evaluations/evaluations.types.ts` 与后端最终 DTO，消除 `undefined` 错误。
- 补充 Demo 模式数据与前端单元测试。

## 非目标

- 不实现完整的用户权限管理 UI。
- 不替代后端 OIDC 服务端点。
- 不修改后端 evaluation 领域模型（由 `backend-response-format-alignment` 负责）。
- 不在本 change 中修改后端 REST 层响应包装格式（仅在前端做兼容）。

## 设计方案

### 1. OIDC 登录入口

新增 `frontend/src/features/auth/LoginPage.tsx`：

- 独立页面，居中展示品牌、说明文字和"使用企业 OIDC 登录"按钮。
- 点击后设置 `window.location.href = "/oauth/oidc/login"`，由后端完成 OIDC 流程。
- 后端 callback 设置 session cookie 后跳转回控制台首页。

在 `AppShell.tsx` 路由表加入：

```tsx
<Route path="/login" element={<LoginPage />} />
```

为提升体验，在 `apiRequest` 中捕获 401 后统一跳转 `/login`：

```ts
if (response.status === 401) {
  window.location.href = "/login";
  throw new Error("未登录");
}
```

Demo 模式下 `/login` 仍渲染，点击跳转回首页并视为已登录（便于离线开发与 e2e）。

### 2. 基准集创建表单

替换 `frontend/src/features/evaluations/EvaluationList.tsx` 中的 `BenchmarkSetForm` 占位实现。

字段：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| name | text | 是 | 基准集名称 |
| description | textarea | 否 | 描述 |
| tasks | textarea | 是 | 任务引用，逗号/换行/空格分隔 |
| is_active | checkbox | 是 | 是否设为当前默认，默认 true |

提交逻辑：

1. 解析 `tasks` 为 `string[]`（trim + 去空）。
2. 调用 `createBenchmarkSet({ name, description, task_refs: tasks })`。
3. 成功后通过 `onCreated` 回调通知父组件刷新列表。
4. 失败时展示错误信息。

表单样式复用现有 `.agent-form`、`.primary-action` 等类名，保持与 `AgentRegister` 一致的视觉风格。

### 3. 类型对齐

新增或调整以下类型定义，统一放到 `frontend/src/features/evaluations/evaluations.types.ts`：

```ts
export type ThresholdResultView = {
  name: string;
  passed: boolean;
  evidence?: Record<string, unknown>;
};

export type EvaluationSummaryView = {
  pass_rate: number;
  avg_latency_ms: number;
  cost_cents: number;
  security_passed: boolean;
  extra?: Record<string, unknown>;
};
```

`BenchmarkSetView` 保持与后端 `BenchmarkSetSummary` 一致：

```ts
export type BenchmarkSetView = {
  id: string;
  tenant_id: string;
  version_number: number;
  name: string;
  description: string;
  is_active: boolean;
  created_by: string;
  created_at: string;
};
```

> 注：后端 `BenchmarkSetSummary.Description` 为 `string` 而非指针，前端类型同步为必填字符串，空值时后端返回 `""`。

`EvaluationRunView` 与 `backend-response-format-alignment` 产出的 `EvaluationRunDetail` 一致：

```ts
export type EvaluationRunView = {
  id: string;
  tenant_id: string;
  agent_version_id: string;
  benchmark_set_id: string;
  status: EvaluationRunStatus;
  environment_digest: string;
  scoring_rule_version: string;
  threshold_results: ThresholdResultView[];
  summary: EvaluationSummaryView;
  started_at: string;
  completed_at?: string;
};
```

创建基准集接口返回的 DTO 与后端保持一致：

```ts
export type CreateBenchmarkSetResponse = {
  benchmark_set_id: string;
  version_number: number;
};
```

为兼容当前后端 `EvaluationRunSummary` 尚未聚合 `summary` / `threshold_results` 的过渡期，组件渲染时做边界处理：

```tsx
const passRate = run.summary?.pass_rate ?? 0;
const thresholds = run.threshold_results ?? [];
```

### 4. API 与 Demo 模式

#### 4.1 响应格式兼容

后端 evaluation 接口当前返回裸数组/对象，而前端 `apiRequest` 统一期望 `Envelope<T>`。在 `evaluations.api.ts` 中引入兼容辅助：

```ts
function unwrapEvaluationResponse<T>(json: unknown): T {
  if (json && typeof json === "object" && "data" in json) {
    return (json as Envelope<T>).data;
  }
  return json as T;
}
```

`listBenchmarkSets`、`createBenchmarkSet` 等函数内部先调用 `apiRequest` 获取响应，再用 `unwrapEvaluationResponse` 提取有效载荷。

#### 4.2 Demo 数据

在 `client.ts` 的 `demo()` 中补充：

- `GET /v1/benchmarks` → 返回示例基准集列表（`Envelope<BenchmarkSetPage>`）。
- `POST /v1/benchmarks` → 解析 body，追加新基准集，返回 `{ data: { benchmark_set_id, version_number }, meta }`。
- `GET /v1/evaluations` → 返回示例评测运行列表（`Envelope<EvaluationRunPage>`）。
- `GET /v1/evaluations/:id` → 返回完整详情（含 `threshold_results` + `summary`）。

### 5. 测试

新增与扩展的测试文件：

- `frontend/src/features/auth/LoginPage.test.tsx`
  - 渲染登录按钮。
  - 点击按钮跳转 `/oauth/oidc/login`。
- `frontend/src/features/evaluations/evaluations.test.tsx`
  - 扩展 `BenchmarkSetForm` 测试：输入字段、提交、校验。
  - 扩展 `EvaluationDetail` 测试：完整数据渲染、缺失 `summary` 与 `threshold_results` 时的边界渲染。
- 运行 `npm test -- --run` 与 `npm run build`。

## 风险与缓解

| 风险 | 缓解 |
|---|---|
| OIDC 登录跳转在本地开发时需要后端已配置 OIDC provider | Demo 模式保留离线可用；真实模式仅在有 OIDC 时测试 |
| `tasks` 输入格式需要与后端 `[]string` 对齐 | 前端支持多种分隔符，提交前统一为 `string[]` |
| 后端 evaluation 接口响应格式过渡期不一致 | `evaluations.api.ts` 中做 Envelope/裸响应兼容，后端对齐后可移除 |
| `EvaluationDetail` 访问未聚合字段导致 `undefined` | 组件层使用可选链与默认值做边界处理 |

## Open Questions

- 登录成功后前端如何感知用户身份？当前设计依赖后端 callback 设置 cookie 后跳转回首页；后续如需展示用户名，可新增 `/v1/session/me` 接口。
- `BenchmarkSetView.description` 后端当前为 `string`（非指针），前端是否应保持可选？为与后端序列化一致，本设计采用必填字符串。

## Spec Patch

已在 `openspec/specs/evaluation/spec.md` 补充：基准集与评测运行接口 SHALL 返回 `Envelope<T>` 结构（`data` + `meta`），与控制台前端 `apiRequest` 约定保持一致。
