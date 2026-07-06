---
comet_change: frontend-capability-gaps
verify_mode: full
verified_at: 2026-07-05
---

# Frontend Capability Gaps — 验证报告

## 验证范围

本次 change 为 full 验证。验证内容包括：

1. tasks.md 全部任务已完成。
2. 实现符合 Design Doc 与 plan。
3. delta spec 与 design doc 无矛盾。
4. 构建与测试通过。
5. 代码审查发现问题已修复。
6. 分支处理结论。

## 验证结果摘要

| 检查项 | 结果 | 证据 |
|---|---|---|
| tasks.md 全部勾选 | PASS | `openspec/changes/frontend-capability-gaps/tasks.md` 中所有 `[x]` |
| 实现符合 Design Doc | PASS | 登录页、路由、401 处理、基准集表单、类型对齐、Demo 数据均已实现 |
| delta spec 一致性 | PASS | `openspec/specs/evaluation/spec.md` 补充了 Envelope 响应要求，Design Doc 中记录了 Spec Patch |
| 前端构建 | PASS | `cd frontend && npm run build` 退出码 0 |
| 前端单元测试 | PASS | `cd frontend && npm test -- --run`：11 个文件，49 个测试全部通过 |
| 后端编译 | PASS | `cd backend && go build ./...` 退出码 0 |
| 代码审查 | PASS | 审查发现 1 个 Important + 4 个 Minor 问题，已全部修复并重新验证 |
| 安全问题 | PASS | 无硬编码密钥、无新增 unsafe 操作、401 跳转已加循环保护 |

## 代码审查结论

审查由独立 subagent 执行，发现以下问题并修复：

1. **Important**: `EvaluationRunView` 的 `summary` 与 `threshold_results` 声明为必填，但组件和测试都按可选处理。已调整为可选类型，并移除了测试中的不安全类型断言。
2. **Important**: 401 跳转缺少循环保护。已增加对 `/login` 和 `/oauth/oidc/login` 的路径判断。
3. **Minor**: `BenchmarkSetForm` 提交按钮未跨列。已添加 `field-span-2`。
4. **Minor**: `LoginPage.test.tsx` 未重置 `vi.stubEnv`。已添加 `vi.unstubAllEnvs()`。
5. **Minor**: `parseTaskRefs` 重复计算。已在组件内复用解析结果。

修复后重新运行 `npm test -- --run` 和 `npm run build`，均通过。

## 环境限制说明

- `make test` 与 `scripts/comet-verify.sh` 中的后端测试需要本地 PostgreSQL/Docker。当前环境 Docker 守护进程处于只读文件系统状态，无法启动测试用 PostgreSQL 容器，导致后端集成测试无法运行。
- 由于本 change 仅修改前端代码，已单独验证：
  - 前端构建与单元测试通过。
  - 后端 `go build ./...` 通过，确认未破坏后端编译。

## 分支处理

- 工作分支：`feature/20260705/frontend-capability-gaps`
- 分支状态：已实现并验证，等待用户决定合并/PR/保留。

## 结论

**验证结果：PASS**

实现符合 Design Doc、plan 与 delta spec，前端构建与测试通过，代码审查问题已修复。建议进入 archive 阶段。
