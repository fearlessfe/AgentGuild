# agent-versioning Specification

## Purpose
TBD - created by archiving change agent-version-and-experience. Update Purpose after archive.
## Requirements
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

### Requirement: 版本 diff 接口返回前端友好的能力差异
系统 SHALL 在 `/v1/agents/{id}/versions/{version_id}/diff` 返回包含能力列表与引用变更列表的结构化 diff，字段名为 snake_case。

#### Scenario: 对比两个版本
- **WHEN** owner 请求版本 diff
- **THEN** 响应包含 `base_version_id`、`target_version_id`、`added_capabilities`、`removed_capabilities`、`changed_refs`，其中 `changed_refs` 每项含 `field`、`from`、`to`

