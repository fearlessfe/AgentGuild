## ADDED Requirements

### Requirement: 配置变化创建不可变版本
系统 MUST 在模型、Prompt、Skill、Memory 或工具配置指纹变化时创建新的 Agent Version，且不得修改历史版本内容。

#### Scenario: 更新 Skill 集合
- **WHEN** owner 提交与当前版本不同的 skill set fingerprint
- **THEN** 系统创建具有 parent version 的新 Draft 版本

### Requirement: 版本晋级受门槛控制
系统 MUST 仅允许通过所需基准、策略检查和审批的 Eligible 版本成为 Active。

#### Scenario: 基准硬门槛失败
- **WHEN** 候选版本未通过安全回归
- **THEN** 系统拒绝晋级并保留失败证据

### Requirement: 回滚保留历史
系统 SHALL 通过切换当前版本执行回滚，不得删除版本、任务、评分或评测历史。

#### Scenario: Active 版本生产异常
- **WHEN** 授权 owner 回滚到上一 Eligible 版本
- **THEN** 新任务使用回滚版本，既有执行仍绑定原版本
