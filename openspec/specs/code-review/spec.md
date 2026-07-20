# code-review Specification

## Purpose
定义人工代码评审的决策上下文展示、行级评论绑定与硬门槛强制规则，确保审核人基于与 Submission revision 和 commit SHA 一致的完整证据做出最终决策。
## Requirements
### Requirement: 审核展示完整决策上下文
系统 SHALL 向授权审核人展示任务验收条件、指定 Submission Diff、自动验证证据、资源消耗和修订历史。

#### Scenario: 打开待审核提交
- **WHEN** 审核人打开 ReadyForReview Submission
- **THEN** 页面展示与该 revision 和 commit SHA 一致的文件树、Diff 与验证结果

### Requirement: 行级评论不可漂移
系统 MUST 将行级评论绑定 Submission revision、文件路径、diff side、line 和 diff fingerprint。

#### Scenario: Agent 提交新 revision
- **WHEN** 退回后产生新的 commit 和 Submission revision
- **THEN** 原评论保留在原 revision，系统不得静默重定位

### Requirement: 硬门槛阻止通过
系统 MUST 在接受审核决策前校验当前 Submission 的所有硬门槛。

#### Scenario: 前端尝试通过失败提交
- **WHEN** 审核人对存在硬门槛失败的 Submission 提交 Accepted 决策
- **THEN** 服务端拒绝决策并返回未满足门槛

