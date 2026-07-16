## ADDED Requirements

### Requirement: 质量门禁 fail closed
系统 MUST 在 Issue 来源 Task 从 draft 发布为 open 前执行不可绕过的质量门禁；任一 hard gate 失败时，无论软评分多高都不得发布。

#### Scenario: 关键证据不可解析
- **WHEN** Task Specification 的关键 path、line span 或 content hash 无法在固定 base commit 解析
- **THEN** 质量门禁失败，Task 保持 draft 并记录证据失败详情

#### Scenario: 软评分高但存在关键歧义
- **WHEN** 规格的综合软评分达到阈值但仍有 unresolved critical ambiguity
- **THEN** 系统不得发布任务，并将其路由到待澄清或人工复核

### Requirement: 验收标准是可判定的一等对象
每条 Acceptance Criterion MUST 具有稳定 ID、statement、criticality、verifier kind、expected result、timeout 和 evidence refs；critical criterion MUST 具有允许的自动 verifier 或明确的人类判定规则。

#### Scenario: 创建可执行命令验收
- **WHEN** 分析结果声明某命令和预期结果用于判断任务完成
- **THEN** 系统将命令、退出状态或结构化期望、超时和证据保存为独立 criterion

#### Scenario: 关键验收只有模糊描述
- **WHEN** critical criterion 仅包含“代码质量良好”或其它无明确判定规则的描述
- **THEN** 质量门禁拒绝该 criterion，并要求改写或人工定义判定规则

### Requirement: 验收检查在 base commit 上资格验证
系统 SHALL 在发布前于固定 base commit 的受控环境中验证所有自动 verifier 可启动、依赖符合策略且观察结果与任务描述一致；适用时缺陷 probe MUST 在 baseline 上失败或观察到预期缺陷。

#### Scenario: 验收命令无法启动
- **WHEN** verifier 命令不存在、依赖未固定、超时或超出 allowlist
- **THEN** 对应 hard gate 失败且任务不得自动发布

#### Scenario: 新增回归 probe 在 baseline 已通过
- **WHEN** criterion 声称 probe 捕获当前缺陷但该 probe 在 base commit 已通过
- **THEN** 系统将 criterion 标记为与 baseline 不一致并阻止自动发布

### Requirement: 质量报告完整可审计
系统 SHALL 为每个 Task Specification Version 保存不可变质量报告，包含 hard gates、软维度、阈值、Analyzer/Critic findings、资格验证证据、人工复核与最终发布决定。

#### Scenario: 查看已发布任务质量依据
- **WHEN** 授权治理者查询已发布任务
- **THEN** 系统返回其发布时规格版本对应的质量报告和每项判定证据

#### Scenario: 质量策略升级
- **WHEN** 部署方发布新质量策略或新 Agent Version
- **THEN** 既有报告保持不变，新分析使用新版本并可与旧结果比较

### Requirement: 公共自动分发具有更严格门槛
系统 MUST 要求公共自动分发任务的全部 critical criteria 可自动判定；包含 critical manual criterion 的任务必须获得授权人类复核后才能公开。

#### Scenario: 公共任务含关键人工验收
- **WHEN** 任务其它 hard gates 均通过但存在 critical manual criterion 且没有人类批准
- **THEN** 系统允许保留私有草稿或待复核状态，但不得进入公共任务池

#### Scenario: 人类批准关键人工验收规则
- **WHEN** 授权复核者确认判定规则、责任人和所需证据
- **THEN** 系统记录批准 provenance，并可继续执行剩余公共发布门禁

### Requirement: 高保证任务支持隔离影子求解
系统 SHALL 支持按质量策略对高风险、高奖励或低置信度任务运行独立 Qualification Solver，并保存其可完成性反馈且不向领取 Agent 暴露影子 patch。

#### Scenario: 影子求解成功
- **WHEN** Qualification Solver 根据公开规格完成 patch 且通过全部自动验收
- **THEN** 系统记录可完成性证据，销毁或隔离 patch，并继续执行其它发布门禁

#### Scenario: 影子求解失败
- **WHEN** Qualification Solver 在预算内无法完成或发现新关键歧义
- **THEN** 系统不得把失败等同于不可解，但必须触发人工复核或保持 draft

### Requirement: 质量效果通过离线与线上指标回归
系统 SHALL 按 Analyzer/Critic/质量策略版本记录 localization、证据覆盖、歧义拒绝、错误发布、错误接受、外部 Agent 完成率、成本和延迟指标。

#### Scenario: 新分析版本准备推广
- **WHEN** 管理员尝试将新分析 Agent Version 或质量策略设为默认
- **THEN** 系统提供其与当前版本在固定历史数据集和 shadow 流量上的对比结果

#### Scenario: 任务生成量上升但错误接受率恶化
- **WHEN** 新版本生成更多可发布任务但 false accept 指标超过治理阈值
- **THEN** 系统不得仅依据任务数量将其判为质量提升
