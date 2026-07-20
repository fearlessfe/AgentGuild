## ADDED Requirements

### Requirement: GitHub App 一键接入
系统 SHALL 提供基于 GitHub App Manifest 的一键接入流程，使人类管理员无需手动输入 App 凭证、平台也无需预置 GitHub App，即可创建并配置本租户的 GitHub App。

#### Scenario: 发起一键接入
- **GIVEN** 已登录的人类管理员
- **WHEN** 触发一键接入入口
- **THEN** 系统以包含所需权限（Contents 只读、Issues 读写、Checks 只读、Metadata 只读）与回调地址、防伪 state 的 App Manifest 跳转至 GitHub App 创建页

#### Scenario: 接入回调换取并持久化凭证
- **GIVEN** 用户在 GitHub 完成 App 创建并被回调带回临时 code 与 state
- **WHEN** 系统收到回调
- **THEN** 系统校验 state，用 code 向 GitHub 换取 App ID 与私钥并持久化到本租户配置，且响应不回显私钥

#### Scenario: state 不匹配拒绝
- **WHEN** 接入回调的 state 与发起时不一致
- **THEN** 系统拒绝该回调且不持久化任何凭证

#### Scenario: 安装回调记录 installation
- **GIVEN** 用户将已创建的 App 安装到组织/仓库并被回调带回 `installation_id`
- **WHEN** 系统收到安装回调
- **THEN** 系统持久化该 `installation_id`，此后可代表该安装访问 GitHub

#### Scenario: 保留手动配置降级
- **GIVEN** 无法使用一键接入回调的环境
- **WHEN** 管理员通过既有手动配置接口提交 App ID、安装 ID 与私钥
- **THEN** 系统持久化配置，效果与一键接入等价

### Requirement: GitHub 连接检测
系统 SHALL 允许人类管理员对已配置的 GitHub App 执行连接检测，验证凭证与安装是否有效。

#### Scenario: 连接检测成功
- **GIVEN** 租户已配置有效 GitHub App 与安装
- **WHEN** 管理员触发连接检测
- **THEN** 系统调用一次轻量 GitHub API 验证并返回成功结果，不回显私钥

#### Scenario: 连接检测失败
- **GIVEN** 租户 GitHub 配置无效或安装已失效
- **WHEN** 管理员触发连接检测
- **THEN** 系统返回结构化失败结果与原因，不泄露敏感凭证

### Requirement: 安装仓库列举
系统 SHALL 允许人类管理员通过已配置的 per-tenant GitHub App，列出该租户可纳入治理的安装仓库。

#### Scenario: 管理员列出安装仓库
- **GIVEN** 租户已配置有效 GitHub App 安装
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回该安装可访问的仓库列表（含名称、默认分支、可见性）

#### Scenario: 未配置 GitHub App
- **GIVEN** 租户未配置 GitHub App
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回明确的未配置错误，且不泄露其它租户信息

### Requirement: 同步规则管理
系统 SHALL 允许人类管理员对每租户的 Issue→Task 同步规则进行创建、查看、更新、删除与启停；规则字段 MUST 至少包含目标仓库、包含标签、排除标签、Issue 状态、任务类型、默认优先级、重复策略与启用状态。

#### Scenario: 创建同步规则
- **WHEN** 管理员以有效字段调用 `POST /v1/sync-rules`
- **THEN** 系统持久化规则并返回其视图，初始可为启用或草稿

#### Scenario: 列出与查看规则
- **WHEN** 管理员调用 `GET /v1/sync-rules`（或按 id 查看）
- **THEN** 系统返回该租户的规则集合/单条规则

#### Scenario: 更新与启停规则
- **WHEN** 管理员更新规则字段或将其启用/停用
- **THEN** 系统持久化变更；停用规则不再参与后续同步

#### Scenario: 删除规则
- **WHEN** 管理员删除规则
- **THEN** 系统移除该规则，后续同步不再执行它

#### Scenario: 非管理员不可写规则
- **GIVEN** 非管理员的人类 session 或 Agent token
- **WHEN** 调用同步规则写接口
- **THEN** 系统拒绝并返回未授权/禁止

### Requirement: Issue 到 Task 周期性同步
系统 SHALL 通过后台轮询按启用的同步规则，从 GitHub 拉取匹配的 Issue 并映射为平台 Task；Issue 来源的 Task MUST 以 system 身份创建，不得要求 Agent publisher。

#### Scenario: 匹配 Issue 生成任务
- **GIVEN** 一条启用规则匹配某仓库中带包含标签、状态匹配的 open Issue
- **WHEN** 同步轮询执行
- **THEN** 系统为该 Issue 创建一个 system 来源的 Task，字段依据规则（任务类型、优先级）与 Issue 内容填充

#### Scenario: 排除标签过滤
- **GIVEN** 某 Issue 同时带有规则的排除标签
- **WHEN** 同步轮询执行
- **THEN** 系统不为该 Issue 创建 Task

#### Scenario: 重复 Issue 去重
- **GIVEN** 某 Issue 已在此前同步中生成过 Task（同 tenant、repo、issue 编号）
- **WHEN** 再次同步该 Issue
- **THEN** 系统不重复创建 Task；依据规则重复策略更新现有 Task 或跳过

#### Scenario: 仅未推进任务可更新内容
- **GIVEN** 某 Issue 对应的 Task 仍处于 open 或 draft
- **WHEN** 同步检测到 Issue 内容变化且规则重复策略为更新
- **THEN** 系统更新该 Task 的内容字段

#### Scenario: 已推进任务不被内容更新覆盖
- **GIVEN** 某 Issue 对应的 Task 已被领取或进入执行/完成/取消
- **WHEN** 同步再次处理该 Issue
- **THEN** 系统仅更新映射元数据，不覆盖该 Task 的状态与内容

#### Scenario: 源 Issue 关闭且任务未领取则取消
- **GIVEN** 某 Issue 对应的 Task 仍处于 open 或 draft
- **WHEN** 同步检测到该 Issue 已关闭/resolved
- **THEN** 系统以 system publisher 身份取消该 Task

#### Scenario: 源 Issue 关闭但任务已领取则仅标记
- **GIVEN** 某 Issue 对应的 Task 已被领取或执行中
- **WHEN** 同步检测到该 Issue 已关闭/resolved
- **THEN** 系统不改变该 Task 状态，仅在来源映射上标记 Issue 已关闭

#### Scenario: 单规则失败不阻塞其它规则
- **GIVEN** 某规则同步时 GitHub 调用失败
- **WHEN** 同步轮询遍历多条规则
- **THEN** 系统记录该失败并继续处理其余规则

### Requirement: 立即同步触发
系统 SHALL 提供由人类管理员手动触发单条规则立即同步的入口，便于验证与冒烟。

#### Scenario: 手动触发同步
- **GIVEN** 一条启用的同步规则
- **WHEN** 管理员调用该规则的立即同步入口
- **THEN** 系统按该规则执行一次同步并返回本次同步结果摘要

### Requirement: Issue 来源任务的读取与来源标识
系统 SHALL 使 Issue 来源的 Task 通过既有任务读取接口对人类 session 可见，并可识别其来源为 GitHub Issue。

#### Scenario: 人类在任务中心看到 Issue 任务
- **GIVEN** 同步已生成 Issue 来源的 Task
- **WHEN** 人类 session 调用 `GET /v1/tasks`
- **THEN** 返回结果包含该 Task，且可辨识其来源为 Issue（关联仓库与 Issue 编号）
