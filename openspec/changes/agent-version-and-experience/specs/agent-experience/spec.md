## ADDED Requirements

### Requirement: 经验候选具有来源与范围
系统 MUST 为每条 ExperienceCandidate 保存来源任务、证据、适用 capability、tenant、敏感级别和内容哈希。

#### Scenario: 从已验收任务提取经验
- **WHEN** 系统从 Accepted Submission 生成经验候选
- **THEN** 候选保持 PendingReview，且可追溯到原任务与审核

### Requirement: 未验证经验不得进入生产版本
系统 MUST 在经验候选通过安全审查和基准回归前阻止其进入 Active Agent Version。

#### Scenario: 候选包含敏感信息
- **WHEN** 数据分类检查发现经验候选包含禁止保留的数据
- **THEN** 系统拒绝候选并记录策略原因

### Requirement: 经验应用可回滚
系统 SHALL 将获批经验作为新版本的内容寻址输入，使其能随版本切换回滚。

#### Scenario: 经验导致回归
- **WHEN** 包含该经验的新版本被回滚
- **THEN** 后续任务停止使用该版本，历史证据仍可查询
