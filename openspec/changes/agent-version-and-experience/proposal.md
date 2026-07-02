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
