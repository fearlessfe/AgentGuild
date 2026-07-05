# Comet Design Handoff

- Change: git-delivery-and-validation
- Phase: design
- Mode: compact
- Context hash: ab1f07effb73c5f73ea495bdbd7646860ff017b513363ca4b874254c0f68245b

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/git-delivery-and-validation/proposal.md

- Source: openspec/changes/git-delivery-and-validation/proposal.md
- Lines: 1-27
- SHA256: bdc17d9cbc366c6ca432748048e417891a98096696217628592391e5944a6bd1

```md
## Why

Coding MVP 必须把 Git commit 作为代码成果的事实来源，并在人工审核前验证提交来源、改动边界和自动检查结果。客户端 Agent 需要通过 MCP 提交 commit 引用与结构化证据，而不是直接上传代码包。

## What Changes

- 增加任务级短期 Git 凭证和受限工作分支。
- 增加 `branch + commit SHA` 成果提交及 Git 仓库真实性校验。
- 增加 Diff、GitHub Checks 和 commit metadata 同步。
- 增加独立 Worker 进程执行构建、测试、静态分析和安全检查。
- 增加 MCP 成果提交与验证状态查询工具，复用 REST 状态机和权限。
- 禁止直接上传代码文件、通用成果包及绕过验证硬门槛。

## Capabilities

### New Capabilities

- `git-delivery`: 定义短期 Git 凭证、受限分支、commit 提交和不可变交付引用；首版实现 GitHub Driver，保留 GitLab Driver 扩展接口。
- `submission-validation`: 定义提交校验、异步验证流水线、结果可见性和硬门槛。

### Modified Capabilities

无。

## Impact

影响 Go Git 集成与 Submission 模块、验证 Worker、PostgreSQL 作业和结果数据、MCP 工具及 GitHub API/Actions 对接。依赖身份和任务生命周期 changes。
```

## openspec/changes/git-delivery-and-validation/design.md

- Source: openspec/changes/git-delivery-and-validation/design.md
- Lines: 1-38
- SHA256: 2cdd5b5c70350de7404ee77007a70d4aa97e65067a893cef82736b37bf4f566c

```md
## Context

Git 仓库（首版为 GitHub）是代码与 commit 的事实来源。AgentGuild 保存任务、交付引用、验证结果和审核证据，但不接收代码压缩包，也不自动合并默认分支。

## Goals / Non-Goals

**Goals:**

- 签发最小权限、短时效、任务级 Git 工作凭证。
- 验证 branch、commit、祖先关系、作者和改动路径。
- 使用独立 Worker 异步运行验证并产出不可变结果。
- 通过 REST 与 MCP 提交同一结构化成果。

**Non-Goals:**

- 通用文件上传、自动 merge、自建 Git 托管和隐藏测试源码披露。

## Decisions

1. API 服务负责签发凭证和创建验证作业；同仓库独立 Worker 进程消费 PostgreSQL 作业表，避免首版引入额外消息中间件。
2. Submission 只接受平台指定 branch、commit SHA、摘要、测试声明和证据；服务端从 GitHub 重新获取事实。
3. 提交接收时冻结 commit SHA 与 diff 指纹；后续发现 force-push 或引用变化时使 Submission 失效。
4. 验证步骤显式建模为 Build、PublicTests、HiddenTests、StaticAnalysis、SecurityScan；硬门槛失败不能进入 ReadyForReview。
5. MCP `submission_create` 与 REST 创建 Submission 共用命令处理器、Lease 校验和 Idempotency Key。

## Risks / Trade-offs

- [Git 平台凭证能力差异] → 抽象 `git.Driver` / `CredentialIssuer`，首版实现 GitHub，预留 GitLab 扩展点。
- [Worker 重复执行] → 作业使用租约、尝试号和幂等结果写入。
- [恶意测试消耗资源] → 隔离 Runner、网络限制、超时和预算熔断。

## Migration Plan

先接入只读 GitHub metadata，再启用凭证签发、Submission 和 Worker；按功能开关逐步启用验证步骤，回滚时停止新作业并保留历史结果。

## Open Questions

- GitHub App 还是 Personal Access Token？需要确认部署环境的组织权限模型。
```

## openspec/changes/git-delivery-and-validation/tasks.md

- Source: openspec/changes/git-delivery-and-validation/tasks.md
- Lines: 1-19
- SHA256: 96cccaf8c11b79cb7d5cac5c97fcf0f988111d4a0bbcacd3fea9eac286b1c605

```md
## 1. Git 交付

- [x] 1.1 实现 Git Driver 抽象、GitHub Client、项目配置和任务级 CredentialIssuer 接口
- [x] 1.2 实现受限 branch 规则、短期凭证签发和撤销
- [x] 1.3 实现 Submission、commit metadata、diff fingerprint 和修订数据模型
- [x] 1.4 实现 commit 存在性、祖先、作者、branch 和 changed path 校验

## 2. 成果接口

- [x] 2.1 实现 REST Submission 创建、查询和幂等处理
- [x] 2.2 实现 MCP `submission_create` 与验证状态查询工具
- [x] 2.3 建立 REST/MCP 成果契约、Lease 和越权测试

## 3. 自动验证

- [x] 3.1 实现 PostgreSQL 验证作业、Worker 租约、重试和取消
- [ ] 3.2 实现 Build、公开测试、隐藏测试、静态分析和安全扫描步骤
- [ ] 3.3 实现日志摘要、资源预算、硬门槛和安全失败输出
- [ ] 3.4 完成 force-push、重复作业、Runner 隔离和失败恢复验收测试
```

## openspec/changes/git-delivery-and-validation/specs/git-delivery/spec.md

- Source: openspec/changes/git-delivery-and-validation/specs/git-delivery/spec.md
- Lines: 1-22
- SHA256: c14520835a163325b220ffb07a2e87696d87f934ae68a1b7c3eac36a294f288a

```md
## ADDED Requirements

### Requirement: 工作凭证最小授权
系统 MUST 为单个 Execution 签发短时效 Git 凭证，仅允许读取指定 repository/base commit 并推送平台指定 branch。

#### Scenario: 推送保护分支
- **WHEN** Agent 使用工作凭证尝试推送默认或保护分支
- **THEN** Git 平台或凭证代理拒绝操作并产生审计事件

### Requirement: Submission 引用可验证 commit
系统 MUST 验证提交 commit 存在于指定 branch、是 base commit 后代、属于当前 Execution 且 changed paths 未越界。

#### Scenario: 提交越界改动
- **WHEN** commit 修改任务 forbidden path
- **THEN** 系统拒绝该 Submission 进入自动验证并返回结构化违规项

### Requirement: MCP 提交结构化成果
系统 SHALL 通过 MCP 接收 `branch`、`commit_sha`、`summary`、`tests` 和 `evidence`，但 MUST NOT 接收代码文件作为正式成果。

#### Scenario: Agent 通过 MCP 提交成果
- **WHEN** Lease 有效的 Agent 调用 `submission_create`
- **THEN** 系统复用 REST 提交校验并返回 Submission 标识与验证状态
```

## openspec/changes/git-delivery-and-validation/specs/submission-validation/spec.md

- Source: openspec/changes/git-delivery-and-validation/specs/submission-validation/spec.md
- Lines: 1-22
- SHA256: f7478617a4a17a503616f7c4d26501adb6dea42ec4e07a328083fde9677c73f7

```md
## ADDED Requirements

### Requirement: 自动验证异步且可追踪
系统 SHALL 为合法 Submission 创建可重试的验证作业，并分别记录各验证步骤、日志摘要、资源消耗和时间。

#### Scenario: Worker 执行验证
- **WHEN** Worker 获取待处理 Submission 作业
- **THEN** 系统按配置运行验证步骤并持久化每步结果

### Requirement: 硬门槛不可绕过
构建、核心测试、路径策略或安全硬门槛失败时，系统 MUST NOT 将 Submission 标记为 ReadyForReview。

#### Scenario: 隐藏测试失败
- **WHEN** 隐藏测试命中硬门槛失败
- **THEN** Submission 进入 ValidationFailed，并只向 Agent 返回安全的失败摘要

### Requirement: 验证结果绑定不可变提交
验证结果 MUST 绑定 commit SHA、diff 指纹、验证配置版本和尝试号。

#### Scenario: 提交后发生 force-push
- **WHEN** 系统检测到 branch 上对应 commit 引用被替换或不再满足关系
- **THEN** 原验证结果不得用于进入人工审核
```

