---
change: frontend-capability-gaps
design-doc: docs/superpowers/specs/2026-07-05-frontend-capability-gaps-design.md
base-ref: 765e03681278108c392e04cbd1dc3175d33c72c4
---

# Frontend Capability Gaps — 实现计划

## 1. 目标与范围

依据 [Design Doc](../specs/2026-07-05-frontend-capability-gaps-design.md) 与任务边界 `openspec/changes/frontend-capability-gaps/tasks.md`，本 change 补齐前端控制台的以下能力：

1. 提供可跳转的 OIDC 登录入口 `/login`，并统一处理 401 跳转。
2. 将 `EvaluationList.tsx` 中的 `BenchmarkSetForm` 从占位实现替换为可真正提交 `POST /v1/benchmarks` 的表单。
3. 对齐 `evaluations.types.ts` 与后端最终 DTO，消除 `EvaluationDetail`/`EvaluationList` 中 `summary` / `threshold_results` 的 `undefined` 错误。
4. 在 `client.ts` 的 Demo 模式中补充 `/v1/benchmarks`、`/v1/evaluations` 相关 mock 数据。
5. 补充 `LoginPage` 与基准集表单的前端单元测试，并确保 `npm test -- --run` 与 `npm run build` 通过。

**本次变更不修改：**
- 后端 OIDC 服务端点与认证流程。
- 后端 evaluation 领域模型与 REST 层字段聚合（由 `backend-response-format-alignment` 负责）。
- 后端响应包装格式（仅在前端做 Envelope/裸响应兼容）。

## 2. 决策依据

- 前端类型统一使用 `snake_case`，与 `frontend/src/api/client.ts` 中的 `Envelope<T>` 约定一致。
- 后端 evaluation 接口当前过渡期可能返回裸对象，因此 `evaluations.api.ts` 中引入 `unwrapEvaluationResponse` 兼容 `Envelope<T>` 与裸响应。
- `BenchmarkSetView.description` 后端为 `string`（非指针），前端同步为必填字符串，空值时由后端返回 `""`。
- `EvaluationRunView` 中的 `threshold_results` 与 `summary` 在过渡期可能缺失，组件层使用可选链与默认值做边界处理。

## 3. 任务拆解

### 3.1 OIDC 登录入口

#### 3.1.1 新增 `frontend/src/features/auth/LoginPage.tsx`
- 独立居中页面：展示品牌、说明文字和“使用企业 OIDC 登录”按钮。
- 非 Demo 模式点击后设置 `window.location.href = "/oauth/oidc/login"`，由后端完成 OIDC 流程。
- Demo 模式下点击跳转回首页（如 `/agents`），便于离线开发与 e2e 测试。

#### 3.1.2 在 `frontend/src/app/AppShell.tsx` 注册路由
- 导入 `LoginPage`。
- 在 `<Routes>` 中加入：
  ```tsx
  <Route path="/login" element={<LoginPage />} />
  ```
  注意放在 `*` catch-all 路由之前。

#### 3.1.3 在 `frontend/src/api/client.ts` 统一处理 401
- 在 `apiRequest` 的响应错误处理中，当 `response.status === 401` 时：
  ```ts
  if (response.status === 401) {
    window.location.href = "/login";
    throw new Error("未登录");
  }
  ```
- 避免在 `/login` 路径触发循环跳转（可选判断）。

**验证命令：**
```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm test -- --run
```

### 3.2 基准集创建表单

#### 3.2.1 替换 `frontend/src/features/evaluations/EvaluationList.tsx` 中的 `BenchmarkSetForm`
- 表单状态字段：
  - `name`（text，必填）
  - `description`（textarea，可选）
  - `tasks`（textarea，必填）
  - `is_active`（checkbox，默认 `true`）
- `tasks` 解析：按逗号 / 换行 / 空格分割，trim 并过滤空字符串，得到 `string[]`。
- 使用 `useMutation` 调用 `createBenchmarkSet({ name, description, task_refs: tasks })`。
- 成功后调用 `onCreated()` 刷新列表，失败时展示错误信息。
- 样式复用 `.agent-form`、`.primary-action` 等，保持与 `AgentRegister` 一致。

#### 3.2.2 更新 `frontend/src/features/evaluations/evaluations.api.ts`
- 引入 `unwrapEvaluationResponse` 辅助函数。
- 调整 `createBenchmarkSet` 返回类型为 `CreateBenchmarkSetResponse`（见 3.3.4）。
- `listBenchmarkSets`、`getBenchmarkSet`、`listEvaluationRuns`、`getEvaluationRun` 均使用 `unwrapEvaluationResponse` 提取有效载荷。

**验证命令：**
```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm run build
```

### 3.3 类型对齐

#### 3.3.1 更新 `frontend/src/features/evaluations/evaluations.types.ts`
- 新增：
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
- 调整 `BenchmarkSetView.description` 为必填 `string`：
  ```ts
  description: string;
  ```
- 调整 `EvaluationRunView`：
  ```ts
  threshold_results: ThresholdResultView[];
  summary: EvaluationSummaryView;
  ```
- 新增创建响应类型：
  ```ts
  export type CreateBenchmarkSetResponse = {
    benchmark_set_id: string;
    version_number: number;
  };
  ```

#### 3.3.2 更新组件边界处理
- 在 `EvaluationList` 中：
  ```tsx
  const passRate = run.summary?.pass_rate ?? 0;
  ```
- 在 `EvaluationDetail` 中：
  ```tsx
  const passRate = run.summary?.pass_rate ?? 0;
  const thresholds = run.threshold_results ?? [];
  ```

**验证命令：**
```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npx tsc --noEmit
```

### 3.4 API 与 Demo 模式

#### 3.4.1 `evaluations.api.ts` 响应兼容
新增辅助函数：
```ts
function unwrapEvaluationResponse<T>(json: unknown): T {
  if (json && typeof json === "object" && "data" in json) {
    return (json as Envelope<T>).data;
  }
  return json as T;
}
```
所有 evaluation API 函数在 `apiRequest` 返回后调用该函数。

#### 3.4.2 `frontend/src/api/client.ts` 补充 Demo 数据
- `GET /v1/benchmarks` → 返回示例 `Envelope<BenchmarkSetPage>`。
- `POST /v1/benchmarks` → 解析 body，追加新基准集，返回：
  ```ts
  { data: { benchmark_set_id, version_number }, meta: demoMeta }
  ```
- `GET /v1/evaluations` → 返回示例 `Envelope<EvaluationRunPage>`。
- `GET /v1/evaluations/:id` → 返回完整详情（含 `threshold_results` + `summary`）。

**验证命令：**
```bash
cd /Users/pengzhen/work/AgentGuild/frontend
VITE_DEMO_MODE=true npm test -- --run
```

### 3.5 测试补充

#### 3.5.1 新增 `frontend/src/features/auth/LoginPage.test.tsx`
- 渲染登录按钮。
- 点击按钮跳转 `/oauth/oidc/login`（非 Demo）或首页（Demo）。

#### 3.5.2 扩展 `frontend/src/features/evaluations/evaluations.test.tsx`
- `BenchmarkSetForm`：
  - 输入 name / description / tasks / is_active。
  - 点击提交，验证 `createBenchmarkSet` 被调用且参数正确。
  - 验证必填校验。
- `EvaluationList` / `EvaluationDetail`：
  - 完整数据渲染。
  - 缺失 `summary` 与 `threshold_results` 时的边界渲染（不抛错）。

**验证命令：**
```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm test -- --run
```

### 3.6 文档与收尾

1. 更新 `openspec/changes/frontend-capability-gaps/tasks.md`：
   - 将 1.1–4.2 中已完成的任务标记为 `[x]`。
2. 运行前端构建：
   ```bash
   cd /Users/pengzhen/work/AgentGuild/frontend
   npm run build
   ```
3. 运行项目级验证：
   ```bash
   cd /Users/pengzhen/work/AgentGuild
   make build
   make test
   ```

## 4. 风险与回退

| 风险 | 缓解措施 |
|---|---|
| OIDC 登录跳转在本地开发时需要后端已配置 OIDC provider | Demo 模式保留离线可用；真实模式仅在有 OIDC 时测试 |
| `tasks` 输入格式需要与后端 `[]string` 对齐 | 前端支持多种分隔符，提交前统一为 `string[]` |
| 后端 evaluation 接口响应格式过渡期不一致 | `evaluations.api.ts` 中做 Envelope/裸响应兼容，后端对齐后可移除 |
| `EvaluationDetail` 访问未聚合字段导致 `undefined` | 组件层使用可选链与默认值做边界处理 |
| 新增 `/login` 路由影响现有默认重定向 | 将 `/login` 放在 catch-all 路由之前，并保留 `/agents` 为默认首页 |

## 5. 验收标准

- `frontend/src/features/auth/LoginPage.tsx` 存在并可渲染。
- `AppShell.tsx` 包含 `/login` 路由。
- `BenchmarkSetForm` 可输入并调用 `createBenchmarkSet`。
- `evaluations.types.ts` 中 `BenchmarkSetView` / `EvaluationRunView` 与 Design Doc 一致。
- `EvaluationList` / `EvaluationDetail` 在完整数据与缺失字段场景下均不抛错。
- `npm test -- --run` 通过。
- `npm run build` 通过。
- `make build` 与 `make test` 通过。
- `openspec/changes/frontend-capability-gaps/tasks.md` 中相关任务已标记完成。
