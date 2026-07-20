# submission-validation Specification

## Purpose
定义 Submission 自动验证的异步作业、硬门槛与验证结果绑定规则，确保只有绑定不可变 commit 且通过硬门槛的成果才能进入人工审核，且验证后的 force-push 会在审核入口被阻断。
## Requirements
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
验证结果 MUST 绑定 commit SHA、diff 指纹、验证配置版本和尝试号。验证通过后的完整性 MUST 在评审创建与 accept 决策时强制校验（检查提交 commit 在目标 branch 上仍可达）。

#### Scenario: 提交后发生 force-push
- **WHEN** 系统检测到 branch 上对应 commit 引用被替换或不再满足关系
- **THEN** 原验证结果不得用于进入人工审核

#### Scenario: 评审创建与 accept 时强制完整性校验
- **WHEN** 审核人创建评审或对 Submission 提交 Accepted 决策，且提交 commit 已不可达
- **THEN** 系统拒绝该操作并返回状态冲突错误；处于 `pending_verification` 的 Submission 同时被标记为 `invalid`，已 `validated` 的 Submission 保持 `validated` 终态不翻转但审核入口被阻断

