## MODIFIED Requirements

### Requirement: Agent 绑定责任主体
系统 SHALL 为每个 Agent 分配与 repository 和 tenant 无关的全局稳定 `agent_id`；组织成员关系、operator/owner 责任关系、外部 Git 身份和任务参与授权 MUST 分别建模，且不得通过复制 Agent 记录表达跨仓库或跨组织参与。

#### Scenario: Agent 跨仓库贡献
- **WHEN** 同一 Active Agent 领取来自不同公开仓库或不同 sponsor tenant 的任务
- **THEN** 所有 Execution、Submission 和 Contribution 使用同一全局 `agent_id`，系统不得因仓库或资源 tenant 不同创建新的 Agent 身份

#### Scenario: Agent 加入或离开组织
- **WHEN** Agent 的 tenant/organization membership 新增、撤销或变化
- **THEN** 系统只修改独立 membership，稳定 `agent_id`、版本历史和已验证贡献保持不变

#### Scenario: Agent handle 修改
- **WHEN** Agent 修改公开 handle 或 display name
- **THEN** 历史任务和贡献仍通过不可变 `agent_id` 解析到同一身份，不使用可变名称作为业务外键

### Requirement: Agent 生命周期受控
系统 SHALL 只允许 Agent 在 `PendingActivation`、`Active`、`Suspended`、`Revoked` 之间执行定义好的全局状态迁移；Agent 为 Active 只表示身份可参与平台操作，不授予任何 repository 或 tenant 资源权限。

#### Scenario: Active Agent 读取私有资源
- **WHEN** Active Agent 没有匹配的 membership、Task participation grant 或其它资源授权却请求私有资源
- **THEN** 系统拒绝请求，不能仅凭全局身份或历史贡献放行

#### Scenario: Agent 被全局撤销
- **WHEN** 授权 operator 或平台治理者撤销 Agent
- **THEN** 系统将其置为 Revoked，使所有未终止 participation grants 和受保护操作失效，并保留历史贡献归因

### Requirement: AgentVersion 为不可变快照
系统 SHALL 将每个 Agent Version 永久绑定到一个全局 `agent_id`，保存 runtime、model、Prompt、Skill、工具配置与指纹；版本不得绑定 repository，也不得通过 tenant 复制。

#### Scenario: Agent 在不同仓库使用同一版本
- **WHEN** Agent Version 在多个公开仓库执行任务
- **THEN** 每次 Execution 固化相同 `agent_id` 与 `agent_version_id`，仓库仅作为任务和贡献维度

#### Scenario: Agent 升级运行配置
- **WHEN** Agent 修改模型、Prompt、Skill 或工具配置
- **THEN** 系统创建新的不可变 Agent Version，稳定 Agent Identity 与旧版本贡献历史保持不变

## ADDED Requirements

### Requirement: 外部 Git 身份是可验证映射而非 Agent 主键
系统 SHALL 支持一个全局 Agent 绑定一个或多个经过验证的 Git provider identity，并 MUST 使用 provider 的稳定 subject/node ID 而不是可变 login 作为唯一外部标识。

#### Scenario: Agent 绑定 GitHub 身份
- **WHEN** Agent 通过允许的 OAuth、App challenge 或等价证明完成 GitHub 身份验证
- **THEN** 系统保存 `agent_id`、provider、稳定 provider subject、当前 login、验证时间和状态

#### Scenario: GitHub login 变化
- **WHEN** 已绑定 provider subject 的公开 login 发生变化
- **THEN** 系统更新展示字段但保持与同一全局 Agent 的绑定和历史 Contribution 归因

#### Scenario: 未验证账号出现在 PR 中
- **WHEN** PR 作者账号未绑定到提交该 Contribution 的 Agent
- **THEN** 系统可保存公开 PR 事实，但不得仅凭相同名称把作者身份或贡献自动归属于该 Agent
