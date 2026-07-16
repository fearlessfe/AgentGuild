## MODIFIED Requirements

### Requirement: 服务端执行 Scope 与资源边界校验
系统 MUST 在每次受保护操作中校验 home tenant、resource tenant、Agent 状态、Scope、repository 范围和适用的任务级 grant，能力声明和公共可见性不得扩大权限。

#### Scenario: 能力匹配但仓库越权
- **WHEN** Agent 声明支持目标语言但请求访问未授权 repository
- **THEN** 系统返回权限错误且不披露目标资源细节

#### Scenario: 公共可见不等于可写
- **WHEN** Agent 能读取公共 Task 摘要但尚未取得该 Task 的 participation grant
- **THEN** 系统拒绝其读取执行私有字段、获取 Git credential 或提交成果

#### Scenario: 资源 tenant 与 home tenant 不同
- **WHEN** 外部 Agent 对公共 Task 执行受保护操作
- **THEN** 系统分别校验 Agent home tenant 身份和 sponsor resource tenant 中的任务级授权，不得把任一 tenant ID 隐式替换为另一方

### Requirement: 仓库范围与通配符
系统 SHALL 支持为 Agent 配置 repository 范围列表，并在资源访问时校验目标仓库是否匹配；公共任务仅可通过匹配的有效 task participation grant 临时授权该任务对应的单个仓库和分支，不得形成通配符扩权。

#### Scenario: Agent 访问未授权仓库
- **WHEN** Agent 请求访问不在其 `repo_scope` 中且没有匹配任务级 grant 的仓库
- **THEN** 系统返回 `FORBIDDEN` 且不披露仓库是否存在

#### Scenario: Grant 临时授权目标仓库
- **WHEN** 外部 Agent 持有某公共 Task 的有效 grant 并请求该 Execution 的 Git credential
- **THEN** 系统仅授权 Task 绑定仓库、平台分支和 base commit，不修改 Agent 的长期 `repo_scope`

## ADDED Requirements

### Requirement: 跨租户任务级 Grant 最小化且短期有效
系统 SHALL 将 task participation grant 绑定 resource tenant、Task、Agent home tenant/ID、Execution、scopes 和 expiry，并 MUST 在 Agent 状态变化、Execution 终止、Task 撤销或策略撤销时停止授权。

#### Scenario: Grant 正常到期
- **WHEN** 数据库时间达到 grant expiry
- **THEN** 系统拒绝后续跨租户读写和凭证签发，且不得依赖客户端本地时间

#### Scenario: Agent 被撤销
- **WHEN** 外部参与 Agent 变为 Revoked
- **THEN** 系统实时校验状态、撤销或视为失效其 grants，并拒绝尚未过期 Token 的任务操作

#### Scenario: Grant 重放到其它 Execution
- **WHEN** Agent 尝试使用一个 Task/Execution 的授权访问另一 Execution
- **THEN** 系统拒绝且记录审计事件

### Requirement: 公共投影与私有资源字段隔离
系统 MUST 为匿名和跨租户主体定义显式字段级公共投影；tenant 配置、私有证据、内部质量日志、私有经验和非必要参与者身份不得因 Task 公开而可见。

#### Scenario: 匿名读取公共任务
- **WHEN** 匿名主体请求已发布公共 Task
- **THEN** 系统仅返回公开标题、摘要、仓库、任务规格公共字段和验收标准

#### Scenario: 外部 Agent 查询内部质量报告
- **WHEN** 外部 Agent 持有执行 grant 但请求仅治理者可见的模型原始输出或安全 finding
- **THEN** 系统拒绝或返回脱敏质量结论，不返回内部内容

### Requirement: 跨租户授权决策完整审计
系统 SHALL 记录所有 task participation grant 的创建、使用、续期、拒绝、撤销和过期事件，审计 MUST 包含 actor、home/resource tenant、Task、Execution、scope、时间与安全原因。

#### Scenario: 管理员调查跨租户访问
- **WHEN** sponsor tenant 授权管理员查询某公共 Task 的访问历史
- **THEN** 系统提供完整 grant 决策链且对非必要 home tenant 数据脱敏

#### Scenario: 越权访问被拒绝
- **WHEN** 外部 Agent 请求 grant 范围外资源
- **THEN** 系统记录稳定原因码，面向 Agent 的错误不得披露目标资源存在性
