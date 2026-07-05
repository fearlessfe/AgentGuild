# Comet Design Handoff

- Change: agent-version-and-experience
- Phase: design
- Mode: compact
- Context hash: 97e0bc12d08a76f9dd2d5d0d5ed87d271604c4aee0776a69c389c3d31142fd5a

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/agent-version-and-experience/proposal.md

- Source: openspec/changes/agent-version-and-experience/proposal.md
- Lines: 1-26
- SHA256: 298b1893ac860ca7020e3cebf81bb6562c7a3977d1853f5121d56597347c7ba9

```md
## Why

Agent 的模型、Prompt、Skill、Memory 或工具配置变化后，其能力与风险可能显著改变。AgentGuild 必须用不可变版本隔离评价，并通过受控的经验候选、回归和晋级机制避免经验污染生产 Agent。

## What Changes

- 增加不可变 Agent Version 和配置指纹。
- 增加版本父子关系、生命周期、晋级、回滚和当前版本切换。
- 增加从已验收任务中提取的经验候选及人工治理。
- 增加基准回归和晋级门槛。
- 禁止评分跨版本合并以及未经验证自动修改生产配置。

## Capabilities

### New Capabilities

- `agent-versioning`: 定义不可变配置快照、版本谱系、状态、晋级和回滚。
- `agent-experience`: 定义经验候选来源、审核、基准验证、应用范围和追溯。

### Modified Capabilities

无。

## Impact

影响 Go Agent Version 与 Experience 模块、PostgreSQL 版本和评测数据、React 版本管理界面及声望查询。依赖身份、任务、Submission、Review 和 Reputation 数据。
```

## openspec/changes/agent-version-and-experience/design.md

- Source: openspec/changes/agent-version-and-experience/design.md
- Lines: 1-37
- SHA256: 85deea266dc8bbb468f7ccccdd9be2afd262a3e4db312b9d7026f0e3a41ad03b

```md
## Context

初始 Agent Version 在激活阶段创建。本 change 补齐后续版本谱系、经验候选、基准回归、晋级和回滚，确保任务评分与运行配置可重现。

## Goals / Non-Goals

**Goals:**

- 对模型、Prompt、Skill、Memory 和工具配置生成不可变版本。
- 从已验收任务提取可审计的经验候选。
- 通过基准回归和明确审批完成晋级或回滚。

**Non-Goals:**

- 自动修改生产 Agent、跨组织经验市场和不可解释的在线自学习。

## Decisions

1. 任何配置指纹变化创建新 Agent Version；已有版本内容不可原地修改。
2. 版本采用父子谱系和状态机：Draft、Evaluating、Eligible、Active、Retired、Rejected。
3. ExperienceCandidate 保存来源任务、证据、适用范围、敏感级别和内容哈希，不直接写入 Agent Memory。
4. EvaluationRun 冻结候选版本、基准集版本、环境与评分规则；晋级必须满足硬门槛并记录审批人。
5. 回滚仅切换 Agent 的 `current_version_id`，不删除失败版本或历史任务关系。

## Risks / Trade-offs

- [配置无法完整复现] → 保存内容寻址引用、哈希、工具版本和环境摘要。
- [经验泄露敏感数据] → 提取前分类、脱敏和人工审批，禁止跨 tenant 使用。
- [基准过拟合] → 使用版本化公开与保留基准，并监控真实任务表现。

## Migration Plan

将现有初始版本标记为 Active，随后启用候选版本、评测和晋级；出现问题时切回上一个 Active 版本并保留完整审计。

## Open Questions

- 初版基准集构建、人工审批角色和各能力门槛在深度设计中确定。
```

## openspec/changes/agent-version-and-experience/tasks.md

- Source: openspec/changes/agent-version-and-experience/tasks.md
- Lines: 1-18
- SHA256: c7486e1c8cea29e8d53384e8b315acca3dafb6839ec0b4ab674c6aed4dfd4b11

```md
## 1. 不可变版本

- [ ] 1.1 实现 Agent Version 指纹、内容引用、父子谱系和状态迁移
- [ ] 1.2 实现候选版本创建、差异展示和历史版本查询
- [ ] 1.3 实现 current version 原子切换、晋级与回滚
- [ ] 1.4 为不可变约束、并发切换和任务版本绑定编写测试

## 2. 经验治理

- [ ] 2.1 实现 ExperienceCandidate 来源、范围、分类、哈希和审批数据模型
- [ ] 2.2 实现从 Accepted Submission 生成候选及敏感数据策略检查
- [ ] 2.3 实现经验审核、拒绝和内容寻址版本引用

## 3. 评测与管理界面

- [ ] 3.1 实现版本化 BenchmarkSet、EvaluationRun、硬门槛和评分规则
- [ ] 3.2 实现 React 版本谱系、候选经验、评测、晋级和回滚界面
- [ ] 3.3 完成经验污染、跨 tenant、基准失败和回滚验收测试
```

## openspec/changes/agent-version-and-experience/specs/agent-experience/spec.md

- Source: openspec/changes/agent-version-and-experience/specs/agent-experience/spec.md
- Lines: 1-41
- SHA256: 9126b40f158fb8e2b2e842770339c258dd4fbb412beecdb4b513526a3723ee87

```md
## ADDED Requirements

### Requirement: 经验候选具有来源与范围
系统 MUST 为每条 ExperienceCandidate 保存来源任务、证据、适用 capability、tenant、敏感级别和内容哈希。

#### Scenario: 从已验收任务提取经验
- **WHEN** 系统从 Accepted Submission 生成经验候选
- **THEN** 候选保持 PendingReview，且可追溯到原任务与审核

#### Scenario: 经验适用范围受 tenant 限制
- **WHEN** 经验候选被创建
- **THEN** 其 `tenant_scope` 与来源 Agent 的 tenant 一致，禁止跨 tenant 查询或应用

### Requirement: 敏感数据候选被自动拒绝
系统 MUST 对经验候选进行数据分类，发现禁止保留的数据时自动拒绝。

#### Scenario: 候选包含敏感信息
- **WHEN** 数据分类检查发现经验候选包含禁止保留的数据
- **THEN** 系统拒绝候选并记录策略原因

#### Scenario: 候选被标记为 forbidden
- **WHEN** `SensitivityPolicy.Classify` 返回 `forbidden`
- **THEN** 候选状态直接变为 `rejected`，不可再被审批

### Requirement: 未验证经验不得进入生产版本
系统 MUST 在经验候选通过安全审查和基准回归前阻止其进入 Active Agent Version。

#### Scenario: 审批通过的经验才能纳入新版本
- **WHEN** owner 创建包含经验候选的新 Draft 版本
- **THEN** 系统仅允许纳入状态为 `approved` 的候选

### Requirement: 经验应用可回滚
系统 SHALL 将获批经验作为新版本的内容寻址输入，使其能随版本切换回滚。

#### Scenario: 经验导致回归
- **WHEN** 包含该经验的新版本被回滚
- **THEN** 后续任务停止使用该版本，历史证据仍可查询

#### Scenario: 回滚后经验不污染旧版本
- **WHEN** 包含新经验的版本被回滚到旧版本
- **THEN** 旧版本的 `memory_ref` / `skill_refs` 不包含被回滚经验
```

## openspec/changes/agent-version-and-experience/specs/agent-versioning/spec.md

- Source: openspec/changes/agent-version-and-experience/specs/agent-versioning/spec.md
- Lines: 1-53
- SHA256: 868780f56d2fa1710d38c47ae1c8585692ae326c14745a11055a7aeb36ce94f9

```md
## ADDED Requirements

### Requirement: 配置变化创建不可变版本
系统 MUST 在模型、Prompt、Skill、Memory 或工具配置指纹变化时创建新的 Agent Version，且不得修改历史版本内容。

#### Scenario: 更新 Skill 集合
- **WHEN** owner 提交与当前版本不同的 skill set fingerprint
- **THEN** 系统创建具有 parent version 的新 Draft 版本

#### Scenario: 相同指纹禁止创建新版本
- **WHEN** owner 提交的配置指纹与当前 Active 版本一致
- **THEN** 系统拒绝创建并提示无变化

#### Scenario: 历史版本不可变
- **WHEN** 用户尝试修改已存在版本的配置内容
- **THEN** 系统拒绝任何 UPDATE 操作并返回不可变错误

### Requirement: 版本状态机受控迁移
Agent Version SHALL 仅允许按 `draft → evaluating → eligible → active` 主线迁移，并支持 `retired` 和 `rejected` 终止状态。

#### Scenario: Draft 进入评测
- **WHEN** owner 对 Draft 版本启动 EvaluationRun
- **THEN** 版本状态变为 `evaluating`

#### Scenario: 评测通过变为 Eligible
- **WHEN** 该版本最新 EvaluationRun 状态为 `passed` 且策略检查通过
- **THEN** 版本状态变为 `eligible`

#### Scenario: 非法状态迁移被拒绝
- **WHEN** 用户尝试将 `draft` 直接置为 `active`
- **THEN** 系统拒绝并返回状态冲突错误

### Requirement: 版本晋级受门槛控制
系统 MUST 仅允许通过所需基准、策略检查和审批的 Eligible 版本成为 Active。

#### Scenario: 基准硬门槛失败
- **WHEN** 候选版本未通过安全回归
- **THEN** 系统拒绝晋级并保留失败证据

#### Scenario: 并发晋级被阻止
- **WHEN** 两个并发事务尝试将不同 Eligible 版本晋级为同一 Agent 的 Active
- **THEN** 仅一个成功，另一个返回状态冲突错误

### Requirement: 回滚保留历史
系统 SHALL 通过切换当前版本执行回滚，不得删除版本、任务、评分或评测历史。

#### Scenario: Active 版本生产异常
- **WHEN** 授权 owner 回滚到上一 Eligible 版本
- **THEN** 新任务使用回滚版本，既有执行仍绑定原版本

#### Scenario: 回滚不修改被回滚版本状态
- **WHEN** owner 执行回滚后
- **THEN** 被回滚版本仍保持 `active` 或 `retired`，其历史记录可查询
```

## openspec/changes/agent-version-and-experience/specs/evaluation/spec.md

- Source: openspec/changes/agent-version-and-experience/specs/evaluation/spec.md
- Lines: 1-45
- SHA256: 6d1f79f2bfe3be9191dad012bdd6fec72be66e12973b05ce34dd881dddbca4c0

```md
## ADDED Requirements

### Requirement: 评测运行冻结候选版本与基准集
系统 MUST 在启动 EvaluationRun 时冻结 `agent_version_id`、`benchmark_set_id`、`environment_digest` 和 `scoring_rule_version`，确保结果可审计、不可被后续配置变更污染。

#### Scenario: 启动 EvaluationRun
- **WHEN** owner 对某个 Draft/Eligible 版本请求启动评测
- **THEN** 系统创建 `EvaluationRun` 记录，状态为 `running`，并锁定上述字段

#### Scenario: 评测运行期间版本被修改
- **WHEN** 某版本已关联处于 `running` 状态的 EvaluationRun
- **THEN** 系统拒绝修改该版本的内容引用或配置字段

### Requirement: 硬门槛决定评测结果
系统 MUST 根据预定义硬门槛判定 EvaluationRun 为 `passed` 或 `failed`，并保存各门槛检查结果。

#### Scenario: 所有硬门槛通过
- **WHEN** EvaluationRun 完成且全部 `threshold_results` 为通过
- **THEN** 评测状态变为 `passed`

#### Scenario: 任一硬门槛失败
- **WHEN** EvaluationRun 完成且至少一项硬门槛失败
- **THEN** 评测状态变为 `failed`，并保留失败证据

### Requirement: 评测结果驱动版本状态
系统 SHALL 根据最新通过的 EvaluationRun 将 Agent Version 推进到 `eligible`；未通过时允许重跑或标记为 `rejected`。

#### Scenario: 评测通过晋级 Eligible
- **WHEN** 某版本最新 EvaluationRun 状态为 `passed`
- **THEN** 该 Agent Version 可被推进为 `eligible`

#### Scenario: 评测失败保留证据
- **WHEN** 某版本 EvaluationRun 状态为 `failed`
- **THEN** 系统保留失败结果，版本可重跑评测或标记为 `rejected`

### Requirement: 基准集版本化
系统 MUST 对 BenchmarkSet 进行版本化管理，同一租户内 `version_number` 单调递增，且支持标记当前默认使用的 Active 基准集。

#### Scenario: 创建新基准集版本
- **WHEN** owner 创建 BenchmarkSet
- **THEN** 系统分配新的 `version_number`，并可选将其标记为 `is_active`

#### Scenario: 使用 Active 基准集自动评测
- **WHEN** 系统配置为自动评测且存在 `is_active=true` 的 BenchmarkSet
- **THEN** 新 Draft 版本创建后可自动使用 Active BenchmarkSet 启动 EvaluationRun
```

