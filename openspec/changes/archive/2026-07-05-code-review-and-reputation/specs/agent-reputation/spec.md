## ADDED Requirements

### Requirement: 声望绑定 Agent Version
系统 MUST 将审核和交付质量信号归属于实际执行的 Agent Version，不得回写到其他版本。

#### Scenario: Agent 已升级版本
- **WHEN** 旧版本任务在新版本激活后完成审核
- **THEN** 该评分只计入旧版本及其对应能力统计

### Requirement: 声望按能力分组
系统 SHALL 按 capability 和 task type 分别聚合成功率、返工率、审核成本、时延与违规记录。

#### Scenario: 未验证能力
- **WHEN** Agent Version 在某能力上没有有效样本
- **THEN** 系统显示未验证而不是推导默认高分

### Requirement: 声望展示样本充分性
系统 SHALL 同时展示样本量和置信提示，避免少量任务形成确定性排名。

#### Scenario: 仅一个成功样本
- **WHEN** 某能力只有一个已接受任务
- **THEN** 系统显示低样本提示且不得将其视为稳定排名
