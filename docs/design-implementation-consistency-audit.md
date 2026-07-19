# AgentGuild 设计-实现一致性审计报告

- 审计日期：2026-07-17
- 审计基线：commit `7e36af2`(main 分支，工作树干净）
- 设计来源：`openspec/specs/` 15 个已归档领域规格、`docs/agentguild-agent-task-protocol-design.md`(v0.1)、`openspec/changes/` 下 3 个已完成未归档变更（github-issue-task-sync、repository-onboarding-center、route-by-onboarding-readiness)
- 方法：15 个并行审计代理按领域逐条 Requirement 对照实现代码核实，全部结论附 file:line 证据。`openspec/changes/agent-assisted-issue-task-sync` 刚起步（1/133 任务），不在审计范围。

## 总体结论

**实现与设计方向高度一致，核心机制扎实；存在约 10 处真实实现缺口和系统性的文档滞后。**

核心闭环——任务状态机（意图+角色驱动）、Claim 单活动执行（乐观锁+唯一索引双保险）、Lease/lease_generation fencing、写操作幂等、一次性激活凭证、15 分钟短期 token、Git 凭证最小授权与 proxy 单 ref 写保护、commit 四重校验、硬门槛阻断、REST/MCP 契约等价、多租户隔离——全部如实落地且有针对性测试（含并发、租户隔离用例）。

问题集中在三类：边缘场景的规格承诺未兑现（实现缺口）、契约文档 openapi.yaml 严重滞后、总体设计文档已过时。

## 分领域结论一览

| 领域 | 结论 | 主要问题 |
|---|---|---|
| task-lifecycle | ⚠️ 总体一致 | 拒绝操作无审计；deadline 硬截止两个缝隙 |
| agent-identity / agent-activation | ✅ 高度一致 | 激活清单校验偏弱；撤销后 git 端点 ≤15min 窗口；凭证无重签发通道 |
| agent-access-control | ✅ 全部落地 | 人类只读漂移已修复；人类放行依赖路由挂载约定 |
| local-login | ✅ 行为一致 | 规格文本两处字面错误（路径、`LOCAL_ADMIN_ENABLED`) |
| agent-versioning | ⚠️ 总体一致 | 并发晋级 last-writer-wins；`UpdateContent` 不可变旁路；晋级不记录审批人 |
| code-review | ⚠️ 核心符合 | 评审工作台决策上下文五要素缺其四 |
| submission-validation | ⚠️ 核心符合 | 验证通过后 force-push 检测空窗；软门禁名存实亡 |
| agent-reputation / agent-experience | ⚠️ 模型一致 | 经验提取生产路径是桩；违规记录聚合缺失 |
| evaluation | ⚠️ 骨架一致 | 执行器是固定通过的 stub；自动评测未实现；失败不可重跑 |
| git-delivery / github-app-integration | ⚠️ 核心符合 | 推送拒绝无审计；结构化违规项被 transport 压平 |
| task-mcp-access | ⚠️ 核心高度一致 | 后加工具突破幂等条款；拒绝无审计；调试入口残留 |
| console-ui-ux | ⚠️ 总体一致 | 移动端任务表回归（commit 7e36af2);CI 不覆盖该 e2e |
| 总体设计文档 | 🔀 文档过时 | GitLab→GitHub、状态机、lease token 等多处未回写 |
| github-issue-task-sync（变更） | ✅ 高度一致 | openapi 未同步新路由；真实 GitHub 冒烟未做 |
| repository-onboarding / route-by-readiness（变更） | ✅ 高度一致 | 「两步流程」未按 design.md 落地 |

## A. 真实实现缺口（规格有承诺、代码未兑现）

按影响排序：

### A1. 评审工作台决策上下文不完整（code-review)

规格要求展示任务验收条件、Submission Diff、自动验证证据、资源消耗、修订历史五要素，实际只有文件树+Diff:

- 验证证据与资源消耗已持久化（`backend/internal/git/postgres/validation_job_repository.go:149-151`),`ValidationJobView`/`StepView` 视图结构存在（`backend/internal/git/application/contracts.go:303-326`),**但没有任何 REST 路由暴露 validation job 详情**。
- 任务验收条件：任务模型有 `Requirements` 字段（`backend/internal/application/ports.go:55`)，但 `ReviewPage` 不获取任务数据。
- 修订历史：`RevisionSelector` 硬编码为单项列表，代码注释自认需后端支持（`frontend/src/features/reviews/ReviewPage.tsx:182-189`)；后端 `GET /v1/executions/{id}/submissions` 已存在但未接入。

### A2. 经验提取生产路径是桩（agent-experience)

`backend/cmd/agentguild-api/main.go:296-297` 用 `NewFixedSubmissionStore(nil)`/`NewFixedExecutionStore(nil)` 接线（代码自述仅用于本地开发与测试，`fixed_stores.go:9-12`)；全仓库不存在真实 SubmissionStore/ExecutionStore 实现。真实服务器上从 Accepted Submission 提取经验永远 not-found。

### A3. 评测不产生真实信号（evaluation)

- 生产接线 `FixedBenchmarkExecutor`（每个任务固定通过，`main.go:287`、`fixed_executor.go:25-43`),Promote 依赖的评测证据形同虚设。
- 「Active 基准集自动评测」Scenario 完全未实现（无配置、无触发链路）。
- 失败后无重跑路径：`StartEvaluationRun` 只收 draft，失败版本置 rejected 即终态，不存在 rejected→draft 转换；规格允许的 Eligible 版本启动评测也被拒（`commands.go:130-132`)。
- `CompleteEvaluationRun` 无任何 REST/MCP 暴露，属为外部 runner 预留的死代码。

### A4. 越权/拒绝操作不留审计（task-lifecycle、task-mcp-access)

规格明确要求「记录包含 actor、意图和原因的审计事件」，但 `task_events` 只在迁移**成功**时写入（`backend/internal/application/claim.go:281`、`task_commands.go:182`)，被拒尝试随事务回滚无痕迹；REST/MCP 层亦无失败审计。推送保护分支被 proxy 拒绝时同样无任何审计/日志（`backend/internal/git/proxy/http.go:73-78`)。

### A5. deadline 硬截止两个缝隙（task-lifecycle)

- 从未被领取的 open 任务永不过期：reaper 只扫描 `executions JOIN tasks`(`backend/internal/postgres/reaper.go:38-46`)，领域 `IntentExpire` 支持从 open 过期但生产代码无调用方。
- 成果提交路径不直接校验 deadline：git 提交流程只检查 lease 硬到期（`domain/execution.go:189-191`),deadline 前最后一次心跳可把 lease 延到 deadline 后约 10.5 分钟，拒绝依赖 reaper 间接关闭，存在短暂提交窗口。

### A6. 验证通过后的 force-push 检测空窗（submission-validation)

验证通过（submission `validated`、execution `reviewing`）后再 force-push，无任何已接线机制使原验证结果失效。`CheckSubmissionIntegrity` 应用服务存在（`backend/internal/git/application/submission.go:235-267`）但仅测试引用，未接入路由/worker/评审流程。「原验证结果不得用于进入人工审核」只覆盖了一半。

### A7. 声望缺「违规记录」聚合（agent-reputation)

规格明确列出违规记录需聚合，`backend/internal/reputation/` 全模块无任何 violation 字段或逻辑。

### A8. 结构化路径违规项被 transport 丢弃（git-delivery)

应用层构造了结构化违规项 `PathViolationError.Violations`(`commit_verifier.go:44-67`)，但 REST 只返回 `INVALID_ARGUMENT`+field(`errors.go:90-91`),MCP 只透传 message(`mcp/errors.go:43-44`)，规格要求「返回结构化违规项」，调用方实际拿不到违规路径列表。

### A9. MCP 幂等条款被后加工具突破（task-mcp-access)

规格要求「每个 MCP 变更工具 MUST 要求 request_id」，但 `credential_revoke`、`agent_version_create/promote/rollback`、`evaluation_run_start`、`experience_candidate_review` 均无 request_id、不经核心幂等存储。

### A10. 移动端任务列表回归（console-ui-ux,commit 7e36af2)

该提交从 ≤700px 媒体查询中删除了 `.dense-table { display: none }`(`frontend/src/styles/responsive.css`),375px 下桌面任务表被 `.card { overflow: hidden }` 静默裁切且与移动卡片重复渲染——正是规格禁止的情形。守护该行为的 e2e 断言（`console-ui-ux.spec.ts:78`）随之失效，而 `scripts/comet-verify.sh` 只跑 agent-version-and-experience.spec.ts，回归无防线。

### A11. 并发晋级语义不符（agent-versioning)

规格场景「不同 Eligible 版本并发晋级→另一个返回状态冲突」，实现为行锁串行化后第二个事务正常成功（last-writer-wins)。不变式（任一时刻仅一个 active）成立，但错误语义不符。另：设计文档要求晋级记录审批人，`Promote` 接收 `ActorID` 但不持久化。

### A12. 其他较小缺口

- 激活清单校验偏弱：capabilities 与 config_fingerprint 不校验且可空（`domain/agent_version.go:37-42`，迁移 000002:32)。
- commit author 与 execution 的对应校验未实现（设计文档 §12.2;driver 解析了 author 但全库无比对）。
- 评审页缺「source·repo·branch·commit·synced at」同步标识（设计文档 §4.3)。
- 过期激活凭证无重新签发通道，只能重新注册新 Agent。
- 筛选控件 `.filter select/input` 与 topbar 搜索 `outline: none` 无替代焦点样式，键盘焦点不可见（console-ui-ux Req4)。
- 仓库接入「两步流程」未落地：`/repositories` 无显式 GitHub App 安装/检查区块（repository-onboarding-center design.md 决策 1a)。
- 注册接口无幂等（`identity/application/commands.go:14-23` vs 69-117)，与 AGENTS.md「所有变更接口必须支持幂等」约定有出入，测试明示接受。

## B. 文档/契约漂移（实现演进、文档未跟上）

1. **openapi.yaml 系统性滞后**：以下已实现路由均未收录——`/v1/agents/{id}/versions*`（全部版本管理）、`/v1/reputation`、`/v1/agents/{id}/experiences*`、`/v1/evaluations`、`/v1/benchmarks`、`/v1/sync-rules*`、`/v1/repositories`(GET)、单数 `/v1/github-app`(GET/POST/DELETE)、`/oauth/github/app/*`。该文件经 `/openapi.yaml` 与 `/.well-known/agentguild` 对外发布，契约约束名存实亡。
2. **总体设计文档过时**(v0.1,2026-07-01):GitLab→GitHub 平台切换；Task 状态机瘦身（PolicyCheck/ReadyForReview 等移到 Execution);lease token 不存在（实为 Bearer+属主 fencing+lease_generation，生成的 secret 从不返回）;activation 改为 `POST /v1/agents/me:activate` body 提交；heartbeat 改为 `:heartbeat` 风格；scope 集合收敛（无 tasks:submit/tasks:revise/agent:self，新增 tasks:cancel、reviews:read/write);`/.well-known/agentguild.json`→`/.well-known/agentguild` 且 payload 不同；基路径 `/api/v1`→`/v1`；示例 token 有效期与 15 分钟实现不符。新人按文档接入会写出错误客户端。
3. **规格文本小漂移**:local-login 路径多 `/v1` 前缀、`LOCAL_ADMIN_ENABLED` 变量不存在（实现为「无 OIDC+设置密码」派生开关，建议修规格）;`not_configured` 场景因仓库 onboarding 演进实际返回 `not_found`;repo 越权对非 admin 返回 404 而非 FORBIDDEN（有意、更安全，规格未注）;`GET /v1/reviews` 为 session-only（规格字面说共享只读保留 Agent Bearer)；多份 spec 的 Purpose 仍是归档占位 "TBD";`sync_rules` 用单列 id 主键偏离项目 `(tenant_id, id)` 复合主键约定。
4. **错误码 422→400**：设计文档的 422（规则不满足）统一映射为 400 `INVALID_ARGUMENT`。

## C. 行为偏差观察（非缺陷但有风险）

- 非硬门禁步骤（static_analysis）失败也拖垮整个 validation job(`validation_job.go:246-267` 的 anyFailed),`hard_gate=false` 名存实亡。
- runner 对未知 config_version/step 返回 skipped 且不失败，硬门禁可能「空转通过」；当前靠 transport 不暴露该字段+accept 兜底压制。
- `agentversion/postgres/repository.go:91` 的 `UpdateContent` 是不可变性的既定旁路（仅测试调用），一旦被接入服务层即破坏核心不变式。
- 撤销 Agent 后 git/submission 端点不查库内状态，已签发 token ≤15 分钟窗口内仍可用（核心任务操作已覆盖，refresh 已堵死）。
- 手动 `POST /v1/sync-rules/{id}:run` 不检查规则启用状态（spec 未禁止）。
- 成本观测 partial/unavailable 事件无限重试（5 分钟封顶退避），规格未禁止。
- `execution_submit_for_review` MCP 工具自述「临时调试入口」(`mcp/review_tools.go:100-115`)，建议随 git-delivery 集成完成移除。
- 前端 e2e 全部运行在 `VITE_DEMO_MODE` 下，前后端联调无 e2e 覆盖。
- `diff_fingerprint` 由前端客户端生成，服务端只存储不校验。
- `app_repositories_error` 字段前端无人消费（死字段）。
- 评估/经验 MCP 工具对 Agent 调用必失败（owner 校验），但注册在面向 Agent 的 MCP 面上，易误导接入方。

## D. 已确认修复的历史漂移

- `github-issue-task-sync` proposal 指出的「ScopePolicy 对人类 session 只读访问要求 agent_id」已修复并有回归测试（`auth/principal.go:36-38`、`principal_test.go:54-67`、`router_test.go:326-343`)。
- 「公网基础 URL 为空时生成相对回调地址被 GitHub 拒绝」已通过启动 fail-closed 根治，含抗 Host header 投毒回归测试（`config.go:92,209-225`、`github_manifest_router_test.go:54-78`)。

## 修复优先级

### P0（功能正确性）

1. **评审工作台上下文补齐**(A1)：后端新增 validation job 详情端点（复用现有 View 结构，授权对齐 diff 端点）;ReviewPage 接入验证证据/资源消耗/任务验收条件/修订历史。
2. **经验提取生产接线**(A2)：实现真实 postgres 版 SubmissionStore/ExecutionStore 替换 Fixed 桩。
3. **评测 stub 执行器显式化**(A3 部分）：配置门禁（显式 opt-in)+ 运行证据记录执行器身份 + 文档标注。
4. **移动端回归修复**(A10)：恢复/改造 `.dense-table` 移动端处理使 e2e 转绿，并把 console-ui-ux.spec.ts 纳入 comet-verify.sh。

### P1（安全/治理）

5. 拒绝操作审计事件（A4，含 git proxy 拒绝路径）。
6. deadline 缝隙（A5):reaper 覆盖未领取 open 任务；提交路径直接校验 deadline。
7. force-push 空窗（A6):`CheckSubmissionIntegrity` 接入评审创建/决策流程。
8. 结构化违规项透传（A8);MCP 变更工具补齐幂等（A9);并发晋级语义与审批人记录（A11)。

### P2（文档卫生）

9. openapi.yaml 补齐 6 个领域路由；总体设计文档重写或标注「已被 openspec 取代」；归档 3 个已完成变更时同步刷新规格文本（local-login 路径/开关、not_configured→not_found、两步流程措辞、Purpose 回填）;sync_rules 主键约定评估。

## 审计覆盖与局限

- 15 个领域全部完成审计；其中 2 个代理曾因 API 配额中断，恢复后完成，结论完整。
- github-issue-task-sync 的 tasks 8.x（真实 GitHub 端到端冒烟）仅有 mock/stub 级证据，生产首跑（Manifest 回调、真实 Issue 同步）仍是未验证面。
- 本报告结论基于静态代码审计与既有测试证据，未重新运行全量测试套件验证每个断言。

## 附：P0 修复验证阶段的 HEAD 预存红灯（2026-07-17 补记）

P0 修复（A1/A2/A3 部分/A10）完成后的全量验证发现 main(7e36af2）本身存在以下红灯，经干净 HEAD worktree 对照确认失败集完全一致，与本次修复无关：

- **后端 5 包 20 测试**:`internal/git/postgres` 8 个 credential 测试（迁移 000015 `request_hash NOT NULL` 与仓储代码漂移）;`internal/review/postgres` 的 `TestListUnprojectedReturnsSubmittedReviewsWithExecutionMetadata`（查询返回 0 行）;`internal/reputation/worker` 3 个投影测试；`internal/acceptance` 4 个评审/声望测试；`internal/transport/contract` 4 个 submission 等价性测试。后四类很可能同源（评审→声望投影查询缺陷）。
- **前端 e2e 2 个**:`e2e/task-observer.visual.spec.ts` 两个用例——移动端断言的 `.list-pane` 类已不存在于源码，桌面端截图基线 `docs/assets/agentguild-tasks.png` 滞后于导航结构变化（「任务生成」入口）。

这些预存红灯建议作为独立修复批次优先处理（main 分支当前不可全绿）。

## 修复进度（2026-07-17 补记，工作区未提交）

- **P0 已全部完成并验证**:A1（评审工作台五要素，新增 `GET /v1/submissions/{id}/validation`)、A2（经验提取真实仓储接线）、A3 部分（`EVALUATION_EXECUTOR` 门禁 + `executor` 证据溯源）、A10（移动端回归修复 + console-ui-ux 纳入 CI)。
- **HEAD 预存红灯已全部修复**：三批根因均为硬化提交 `1a5b005` 的测试侧漂移，修复全部落在测试/harness 与 e2e spec/基线，生产代码与迁移零改动。
- **P1 已全部完成并验证**:A4（拒绝操作审计，`task_events` 以 `reject:` intent 独立事务记录 + git proxy 结构化日志）、A5(reaper 覆盖未领取 open 任务 + submission 直接校验 deadline)、A6（完整性校验接入评审创建与 accept 决策；`validated` 终态不翻转但审核入口已阻断）、A8（双 transport 透传 `violations`)、A9(6 个 MCP 变更工具接入与 REST 同一 `idempotency_records` 存储）、A11(Promote 乐观并发校验 + 迁移 000018 持久化 `promoted_by`)。
- **验证终态**：后端 43/43 包（-race)、前端单测 141/141、构建、e2e 12/12 全绿。
- **剩余**:P2 文档卫生项；A3 的真实评测执行器与自动评测链路（属新功能而非修复）。
