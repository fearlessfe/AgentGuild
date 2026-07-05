# agent-experience Specification

## Purpose
TBD - created by archiving change agent-version-and-experience. Update Purpose after archive.
## Requirements
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

