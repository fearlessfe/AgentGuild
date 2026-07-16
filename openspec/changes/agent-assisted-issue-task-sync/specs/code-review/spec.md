## MODIFIED Requirements

### Requirement: 审核展示完整决策上下文
系统 SHALL 向授权审核人展示 Execution 领取时绑定的 Task Specification Version、逐条验收条件、指定 Submission Diff、自动验证证据、资源消耗、源 Issue/base commit、所用经验版本和修订历史。

#### Scenario: 打开待审核提交
- **WHEN** 审核人打开 ReadyForReview Submission
- **THEN** 页面展示与该 revision、commit SHA 和绑定规格版本一致的文件树、Diff、验收标准与验证结果

#### Scenario: Task 已产生更新规格
- **WHEN** Submission 进入评审前 Task 因 Issue 更新产生了新的 current specification version
- **THEN** 审核界面仍以 Execution 绑定版本作为决策契约，并明确提示存在较新版本

### Requirement: 硬门槛阻止通过
系统 MUST 在接受审核决策前校验当前 Submission 对其绑定 Task Specification Version 的所有 critical Acceptance Criteria 和既有验证硬门槛；任一失败、缺失或未判定都阻止 Accepted。

#### Scenario: 前端尝试通过失败提交
- **WHEN** 审核人对存在硬门槛失败的 Submission 提交 Accepted 决策
- **THEN** 服务端拒绝决策并返回未满足门槛

#### Scenario: 关键验收缺少证据
- **WHEN** 某 critical criterion 没有成功自动验证或所需人工判定证据
- **THEN** 服务端拒绝 Accepted，并返回 criterion ID 与安全的缺失原因

#### Scenario: 非绑定规格的验证通过
- **WHEN** Submission 仅通过 Task 新版本的验证但未满足 Execution 绑定版本
- **THEN** 系统不得使用新版本结果绕过原执行契约

## ADDED Requirements

### Requirement: 评审结论逐条关联验收标准
系统 SHALL 要求 Review 对每个 Acceptance Criterion 记录 satisfied、failed、not_applicable 或 needs_evidence 状态、说明和 evidence refs；critical criterion 不得以 not_applicable 绕过，除非授权治理者记录规格修订原因。

#### Scenario: 审核人接受全部关键标准
- **WHEN** 审核人为所有 critical criteria 提交 satisfied 判定和所需证据
- **THEN** 系统保存逐条结果，并在其它硬门槛通过后允许 Accepted 决策

#### Scenario: 审核人认为验收标准本身错误
- **WHEN** 审核发现某 critical criterion 与 Issue 或仓库事实冲突
- **THEN** 系统要求 RequestRevision、拒绝或启动任务规格治理流程，不得直接标记 not_applicable 后接受

### Requirement: 验收证据绑定 Submission 与规格版本
自动和人工验收证据 MUST 绑定 tenant、Task、Execution、Submission revision、commit SHA、Task Specification Version、criterion ID、验证环境和时间，且不得在新 revision 上静默复用。

#### Scenario: Agent 提交新 revision
- **WHEN** 返工产生新的 commit 和 Submission revision
- **THEN** 原 criterion evidence 保留在原 revision，新 revision 重新执行适用验证

#### Scenario: 验证环境不可复现
- **WHEN** 某 evidence 缺少固定镜像、命令、输入或结果摘要
- **THEN** 系统将其标记为不完整，且不得用它满足 critical criterion

### Requirement: 评审结果反馈仓库经验候选
系统 SHALL 在 Review 达到终态后，将逐条验收结果、拒绝原因和验证证据作为 repository experience extraction 的受控输入；Review 本身不得直接创建 active 经验。

#### Scenario: Submission 被接受
- **WHEN** Review 接受通过全部关键验收的 Submission
- **THEN** 系统可幂等创建成功经验候选，引用 Task、规格、Submission、Review 和验证证据

#### Scenario: Submission 因明确方案失败被拒绝
- **WHEN** Review 记录可归因的技术失败和相关证据
- **THEN** 系统可创建负面经验候选，仍需经过经验晋升策略后才能 active
