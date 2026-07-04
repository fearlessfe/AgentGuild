# Brainstorm Summary

- Change: gitlab-delivery-and-validation
- Date: 2026-07-03

## 已确认上下文

- OpenSpec canonical spec 已通过 handoff 脚本生成到 `openspec/changes/gitlab-delivery-and-validation/.comet/handoff/design-context.md`。
- Change 原始目标是让 GitLab commit 成为代码成果事实源；brainstorming 已确认首版改为 GitHub-first，Agent 通过 REST/MCP 提交 PR/commit 引用与结构化证据，AgentGuild 获取 GitHub PR diff 和 checks 结果展示。
- 现有后端已有 `tasks`、`executions`、`idempotency_records`、`task_events`、`outbox_events`、reaper worker、REST 和 MCP 传输层。
- 现有应用边界采用 `application.Service` + `Store.WithTx` + repository ports，REST/MCP 都通过共享 application service 消费能力。
- 当前未提交改动只属于 `agent-onboarding-and-identity` 的 build 进度，不纳入本 change 设计。

## 候选设计方向

- 候选：新增 delivery/submission 领域模型，和现有 Task/Execution 状态机通过 `execution_id` 关联；提交后把 Execution 从 `running` 推进到 `submitted` / `validating` / `reviewing`。
- 候选：用 GitHub checks sync 替代自有验证 worker；保留 provider-neutral `ValidationEvidence` / `CheckRun` 模型，首版 evidence 来自 GitHub Actions/check runs。
- 候选：Git provider 集成通过接口隔离具体凭证策略，先设计 `CredentialIssuer` 与 `GitProviderClient` ports，GitHub 作为首个 adapter。

## 已确认设计方向

- 用户确认将首版方向从 GitLab-first 调整为 GitHub-first + Git provider abstraction。
- 当前 change 继续推进，但需要在 design 输出中包含 Spec Patch，把 GitLab 专用表述调整为 provider/GitHub-first；GitLab 后续作为可插拔 adapter。
- GitHub 首版凭证机制确认采用 GitHub App installation token；设计可保留本地/测试用 PAT fake adapter，但生产主路径按 GitHub App。
- 用户确认交付工作流走 Pull Request，而不是仅提交同仓库任务分支。
- 用户确认 PR 由 Agent 创建；AgentGuild 接收/获取对应 PR，在页面渲染 PR diff，并支持在 diff 上评论。
- 用户确认 diff 行评论首版只保存在 AgentGuild，不同步回 GitHub PR。
- 用户确认 AgentGuild 首版不验证 CI 结果、不自建 CI worker；直接获取 GitHub 上的 Actions/check results 并展示。
- 用户确认 GitHub checks 在 AgentGuild 中只作为展示信息，不作为 Submission/Execution 状态门槛。
- 用户提出新增 GitHub issue 定时同步入口：输入 GitHub 地址后，AgentGuild 定时获取对应 issues 并生成任务；Agent 完成后提交 PR，AgentGuild 可获取并展示对应 PR 信息。
- 用户确认 issue 同步首版采用“指定 repo + label filter”；管理员可以在 AgentGuild 添加/管理可同步的 GitHub repo。
- 用户确认 issue 同步采用“只创建和更新，不自动取消进行中任务”：新匹配 issue 创建 Task，issue 更新同步任务来源快照；issue close 或 label 移除时，未领取任务可取消/隐藏，已领取或进行中的任务只展示 source warning，不自动取消。
- 用户确认采用方案 A：Provider-neutral 领域 + GitHub-first adapter + polling MVP。
- 用户以“继续”确认设计段 1：Provider-neutral delivery 模块边界，GitHub 作为首个 adapter，AgentGuild 保存 PR diff/comment/check 展示数据。

## 待确认问题

- GitHub issue/PR/checks 同步首版采用 polling-only、webhook-only，还是 polling MVP + webhook extension 的架构。

## 关键取舍与风险

- Git provider 最小权限能力受 provider 权限模型限制，设计需要保留可替换的 `CredentialIssuer`。
- GitHub-first 首版需要避免把领域命名绑死在 GitHub；应将 GitHub 放在 adapter/config/credential 层，Submission 和 Validation 保持 provider-neutral。
- 采用 Provider-neutral 领域 + GitHub-first adapter 后，核心表/DTO 使用 provider/repository/issue/pr/check 命名，GitHub 细节隔离在 adapter 与配置中。
- 模块边界确认：GitHub adapter 负责 GitHub App token、issue polling、PR metadata/diff/checks 拉取；应用服务负责权限、幂等、Task 创建、Submission 校验和页面 DTO；polling worker 只同步外部事实，不执行 CI。
- GitHub App installation token 可按 installation/repository 授权并短期过期，但不能天然做到每个 token 只允许一个分支；分支边界需要通过平台指定 branch、服务端提交校验、branch protection/ruleset 或代理策略补齐。
- PR 工作流需要把交付事实扩展为 provider、repository、base branch/base SHA、work branch、head SHA、PR number/URL、diff fingerprint 和 revision；GitHub checks 展示必须绑定不可变 head SHA，而不能只绑定 PR 当前 head。
- 因 Agent 创建 PR，GitHub App token 需要 `contents:write` 与 `pull_requests:write`，AgentGuild 必须校验 PR 属于指定 repo、base branch/base SHA、head branch/head SHA、当前 Execution 和允许的 changed paths。
- 页面 diff 评论与 `code-review-and-reputation` change 有重叠；本 change 应至少产出可评论的不可变 diff/revision 锚点，完整审核决策与声望仍应留给后续 review change。
- 评论只保存在 AgentGuild 时，需要保存 GitHub diff anchor（provider、repo、pr_number、head_sha、file path、side、line/position、diff hunk/fingerprint）以支持后续 revision/outdated 状态与可选 GitHub 同步。
- 不自建 validation worker 后，需要避免把 GitHub PR 最新 check 结果误用于旧 submission；check suites/runs/statuses 必须绑定 provider、repo、pr_number、head_sha、run/check identifiers 和同步时间。
- GitHub checks 日志/annotation 链接可展示摘要与外链；首版不拉取完整 CI 日志，不处理隐藏测试源码或安全摘要生成。
- GitHub checks 只展示时，Submission 状态可在 PR/commit/path 校验通过后进入 `submitted`/`reviewing`，checks failure 只影响页面提示和人工判断，不阻断评论。
- GitHub issue sync 会新增上游 task source：需要保存 repo URL/installation、issue number、issue URL、issue etag/updated_at、label/filter、sync cadence、last cursor，并用 provider issue identity 做幂等 Task 创建。
- Issue sync 与 PR submission 需要保持可追溯闭环：Task 记录 source issue，Submission 记录 target PR，页面可从 Task 看到 issue 和 PR/checks/diff/comments。
- Repo 管理应走管理员权限：管理员添加 repo/provider installation、base branch、允许路径、label filter、同步间隔和启停状态；普通 Agent 只能消费由 repo sync 生成的任务或按权限提交 PR。
- Issue close/label removed 不应强制取消已领取/进行中 Task；这避免外部 triage 操作破坏 Agent lease/execution，一切自动取消只限未领取或未开始的任务。

## 测试策略候选

- 领域测试覆盖 Submission 状态、revision、diff fingerprint、GitHub check evidence 和不可变结果绑定。
- PostgreSQL 集成测试覆盖 PR/commit/changed path 校验、check sync 幂等、force-push 后 revision 隔离。
- REST/MCP 合约测试覆盖同一命令处理器、幂等键、Lease 校验、越权和错误映射。
- GitHub provider client 使用接口假实现和少量契约测试隔离外部 GitHub 不稳定性。
- Issue sync 测试覆盖定时 polling、重复同步幂等、label/filter、关闭/重开 issue 行为和 Task 外部来源映射。

## Spec Patch 候选

- 候选：将 GitLab-specific wording 调整为 Git provider / GitHub-first，包括 proposal、design、`gitlab-delivery` capability 命名、凭证与分支保护场景。用户已确认方向，待最终设计确认后回写。
- 候选：将 `submission-validation` 从“AgentGuild 执行验证作业”改为“同步 provider CI/check results 并展示”，删除或弱化内部 worker/隐藏测试/资源预算要求。
- 候选：新增或调整 spec，覆盖 GitHub issue source sync：输入 repo/issue 地址或 repo sync 配置，定时导入 issues 为 tasks，并保持 issue→task→PR 闭环。
- 待确认是否需要补充“凭证签发失败/部署不支持细粒度凭证时必须降级为不可用而不是扩大权限”的验收场景。
- 待确认是否需要补充“force-push 后旧 check 结果不得复用到新 revision”的验收场景。
