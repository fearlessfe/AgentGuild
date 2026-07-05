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
