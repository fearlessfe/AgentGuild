# route-by-onboarding-readiness 验证报告

- 日期：2026-07-12
- Change：`route-by-onboarding-readiness`
- 验证模式：full
- 基线：`1f1949c6611036a0c1a16a1ff5cc565b058f85ee`
- 最终实现提交：`3b970ca6d4eac6156aa61fc385d7072df7afafdd`

## 摘要

| 维度 | 结果 |
|---|---|
| Completeness | PASS：5/5 OpenSpec tasks 完成；2/2 requirements 已实现 |
| Correctness | PASS：8/8 scenarios 有实现与直接测试证据 |
| Coherence | PASS：实现遵循 OpenSpec design 与技术 Design Doc，无漂移 |
| Build | PASS：后端 `go build ./...`、前端 TypeScript/Vite build 通过 |
| Tests | PASS：后端全量、前端 16 files/82 tests、1 个 Playwright E2E 通过 |
| Review | PASS：最终 standard 复查为 0 Critical、0 Important、0 Minor |

## 完整性

- `openspec instructions apply` 报告 5 项任务全部完成，remaining 为 0。
- `Default console entry follows onboarding readiness` 已由 `HomeRedirect`、路由配置和登录跳转实现。
- `GitHub App manifest uses an explicit public origin` 已由配置解析、manifest 数据流和部署文档实现。
- 未新增 REST/MCP API、数据库迁移或依赖。

## 正确性与场景覆盖

### 控制台入口

- GitHub App 已配置、`installation_id > 0` 且已接入仓库非空时进入 `/tasks`。
- App 未配置、未安装或无已接入仓库时进入 `/onboarding`。
- readiness 请求失败时保守进入 `/onboarding`。
- `/agents`、`/tasks`、`/onboarding` 显式路由保持可访问，不触发隐式判定。
- `/`、本地登录成功和未知路径统一经过 `HomeRedirect`，导航使用 replace。

### GitHub App 公网 origin

- `WEB_ENABLED=true` 时缺失变量会在配置加载阶段失败。
- `WEB_ENABLED=false` 时变量可为空。
- 拒绝相对 URL、非 HTTP(S) scheme、缺 host、userinfo、子路径、query、fragment，以及空 `?`/`#` 分隔符。
- 唯一允许的末尾 `/` 被规范化为无斜杠 origin。
- 带冲突 `Host` 与 `X-Forwarded-*` 的真实 router 测试证明 manifest 的 URL、callback、setup URL 仍只来自显式配置。

## 设计一致性

- 前端复用 `GET /v1/repository-onboarding`，没有新增 onboarding 状态接口或本地持久化。
- 就绪规则封装为纯函数，React 组件只负责 query 状态与导航。
- 后端在 `config.Load` 边界校验 origin；`ManifestService` 不接收请求对象或请求头。
- 配置校验、前端分流、错误降级与文档均符合技术 Design Doc。
- Delta specs 与 Design Doc 无矛盾或实现偏差。

## 最新验证证据

最终修复后运行：

```text
GOCACHE=/tmp/agentguild-go-cache make build
scripts/comet-verify.sh
```

结果：

- `make build` exit 0；Go build 与前端 build 成功。
- `scripts/comet-verify.sh` 在获准访问本地测试端口与 Docker/PostgreSQL 的环境中 exit 0。
- 后端所有包通过。
- Vitest：16 个文件、82 个测试全部通过。
- Playwright：`agent-version-and-experience.spec.ts` 1/1 通过。
- `openspec validate route-by-onboarding-readiness --strict` 通过。

受限沙箱内首次运行全量脚本时，`httptest` 本地端口和 Docker socket 被拒绝；根因确认是环境权限限制，同一脚本在获准环境原样重跑通过，没有修改源码规避测试。

## 安全检查

- 无硬编码密钥、私钥或真实凭证。
- 不信任 Host/forwarded headers 生成 OAuth 回调，缩小 Host header poisoning 风险。
- 错误消息只包含配置变量名与格式要求，不回显 secret。
- 未扩大租户、Agent scope 或仓库授权边界。

## 代码审查

首次 standard 审查发现 2 个 Minor：空 query/fragment 分隔符和边界测试覆盖。修复提交 `3b970ca6` 补齐实现与测试；最终复查结果为：

- Critical：0
- Important：0
- Minor：0
- Assessment：Ready to merge

## 最终结论

所有完整性、正确性、设计一致性、构建、测试与安全检查均通过。Change 已满足归档前的技术验证条件。
