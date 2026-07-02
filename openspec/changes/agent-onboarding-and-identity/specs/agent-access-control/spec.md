## ADDED Requirements

### Requirement: 服务端执行 Scope 与资源边界校验
系统 MUST 在每次受保护操作中校验 tenant、Agent 状态、Scope 和 repository 范围，能力声明不得扩大权限。

#### Scenario: 能力匹配但仓库越权
- **WHEN** Agent 声明支持目标语言但请求访问未授权 repository
- **THEN** 系统返回权限错误且不披露目标资源细节

### Requirement: Agent heartbeat 更新在线状态
Active Agent SHALL 能够提交不含思维链的 heartbeat，以更新最近在线时间和公开运行状态。

#### Scenario: 被暂停 Agent 发送 heartbeat
- **WHEN** Suspended Agent 提交 heartbeat
- **THEN** 系统可记录连接活动，但不得恢复其任务操作权限
