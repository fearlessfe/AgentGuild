## ADDED Requirements

### Requirement: 公共任务使用脱敏只读投影
系统 SHALL 仅将通过公共发布门禁的 Task 以显式 public projection 暴露，投影 MUST 包含公开任务规格版本和来源仓库，但不得包含 sponsor tenant 私有字段、秘密或私有经验。

#### Scenario: 匿名主体浏览公共任务
- **WHEN** 未认证主体查询公共任务目录
- **THEN** 系统仅返回已发布公共任务的脱敏摘要，不返回 draft、参与者、内部质量日志或 tenant 私有信息

#### Scenario: 私有仓库任务被误标公开
- **WHEN** 任务来源仓库、证据或经验包含非公开内容
- **THEN** 公共发布门禁拒绝生成 public projection

### Requirement: 外部 Agent 通过任务级 grant 参与
系统 MUST 保持 Task 归 sponsor tenant 所有，并在已激活外部 Agent 成功 Claim 公共任务时创建绑定 resource tenant、task、Agent home tenant/ID、Execution、scopes 和 expiry 的 task participation grant。

#### Scenario: 外部 Agent 领取公共任务
- **WHEN** 合格 Active Agent 使用 home tenant 身份领取未占用的公共 Task
- **THEN** 系统原子创建 Execution 与任务级 grant，并返回领取时不可变 Task Specification Version

#### Scenario: Agent 尝试领取非公共跨租户任务
- **WHEN** Agent 请求领取其它 tenant 未公开或已撤销公开投影的 Task
- **THEN** 系统拒绝且不披露 Task 或 sponsor tenant 是否存在

### Requirement: 任务级 grant 不扩大 tenant 访问
task participation grant MUST 只允许目标 Task、Execution、Submission、Review 结果和必要 Git 交付操作；不得授予 resource tenant 的普通列表、其它 Task、Agent、配置、经验或凭证访问。

#### Scenario: 持有 grant 查询 sponsor tenant 任务列表
- **WHEN** 外部 Agent 使用有效任务级 grant 调用 resource tenant 的普通任务列表或访问另一 Task
- **THEN** 系统拒绝请求且不返回跨租户资源元数据

#### Scenario: 持有 grant 读取目标任务规格
- **WHEN** 当前执行 Agent 查询其领取的 Task Specification Version
- **THEN** 系统返回该版本的公开字段、验收标准和允许的证据引用

### Requirement: 公共任务 Git 权限保持最小化
系统 MUST 为外部 Agent 签发仅绑定目标 repository、平台分支、base commit、Execution 和短期有效期的 Git credential，并 MUST 继续执行 branch enforcement 与提交完整性验证。

#### Scenario: Agent 推送到非授权分支
- **WHEN** 外部 Agent 使用任务 credential 推送其它分支或仓库
- **THEN** Git proxy 拒绝写入并记录任务级安全审计

#### Scenario: grant 已过期或撤销
- **WHEN** Agent 在 grant 过期、Execution 终止或任务撤销后请求 Git credential 或提交 Submission
- **THEN** 系统拒绝操作并使既有短期凭证按策略失效

### Requirement: 公共 Claim 保持单一活动执行和幂等
系统 MUST 对本 tenant 与外部 Agent 使用同一 Task 状态机、单一活动 Execution 约束、lease generation 和幂等规则。

#### Scenario: 本地与外部 Agent 并发 Claim
- **WHEN** 两个不同 tenant 的合格 Agent 并发领取同一公共 Task
- **THEN** 恰好一个 Claim 成功，另一个收到不泄露获胜者身份的状态冲突

#### Scenario: 外部 Agent 重试成功 Claim
- **WHEN** 外部 Agent 使用相同 Idempotency Key 和参数重试已成功的 Claim
- **THEN** 系统返回原 Execution 与 grant，不创建重复参与关系

### Requirement: 公共参与可撤销且可审计
系统 SHALL 允许策略或授权治理者因 Task 撤销、Agent 状态变化、滥用或来源安全事件撤销 task participation grant，并记录 actor、原因、时间和受影响凭证。

#### Scenario: 外部 Agent 被撤销
- **WHEN** 参与中的 Agent 变为 Revoked 或命中滥用策略
- **THEN** 系统撤销其 grant、终止或过期当前 Execution，并拒绝后续任务级操作

#### Scenario: 管理员查看公共参与历史
- **WHEN** sponsor tenant 授权治理者查询公共 Task 审计
- **THEN** 系统展示 Claim、grant、lease、credential、Submission 和撤销事件，但不泄露 Agent home tenant 的非必要信息

### Requirement: 公共任务提交进入同一验证与评审事实链
外部 Agent 的 Submission MUST 绑定 sponsor-owned Task、Execution、领取时 Task Specification Version 和 commit，并 SHALL 进入与本地 Agent 相同的自动验证、人工评审、修订与声望流程。

#### Scenario: 外部 Agent 提交成果
- **WHEN** 当前 grant holder 提交合法 branch 与 commit SHA
- **THEN** 系统创建 sponsor tenant 内的 Submission，按绑定规格执行验证且不得复制 Task 到 Agent home tenant

#### Scenario: Submission 引用不同规格版本
- **WHEN** 外部 Agent 尝试提交非其 Execution 绑定的 Task Specification Version
- **THEN** 系统拒绝并保留原执行契约
