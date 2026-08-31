# agent-activation Specification

## Purpose
定义 Agent 激活流程：一次性激活凭证的签发与消费、激活时创建初始不可变 Agent Version，以及短期 Access Token 的签发与刷新规则。
## Requirements
### Requirement: 激活凭证单次有效
系统 MUST 签发有过期时间、绑定预注册 Agent 且最多使用一次的 Activation Token。

#### Scenario: 重放激活请求
- **WHEN** 客户端再次使用已成功消费的 Activation Token
- **THEN** 系统拒绝请求，且不创建第二个 Agent Version 或访问令牌

#### Scenario: 过期 Activation Token
- **WHEN** 客户端使用已过期的 Activation Token
- **THEN** 系统拒绝请求并提示需要重新签发凭证

### Requirement: 开放注册挑战单次有效
系统 MUST 将开放注册 challenge 绑定到提交的 Ed25519 公钥，并设置短期过期时间和最多一次消费语义。

#### Scenario: 有效注册证明
- **WHEN** Agent 使用 challenge 对应公钥的私钥签署规范化注册消息
- **THEN** 系统验证签名并在创建身份的同一事务中消费 challenge

#### Scenario: challenge 重放或公钥替换
- **WHEN** Agent 重放已消费的 challenge，或用不同公钥提交签名
- **THEN** 系统返回统一的无效注册证明错误，且不创建任何身份、版本、membership 或访问令牌

### Requirement: 激活生成不可变版本
系统 SHALL 校验 Agent 上报的 runtime、model、capabilities 和配置指纹，并创建初始不可变 Agent Version。

#### Scenario: 成功激活
- **WHEN** 合法 Token 对应的 Agent 提交有效运行清单
- **THEN** 系统原子地创建初始版本、激活 Agent 并签发短期访问令牌

### Requirement: 访问令牌刷新
系统 SHALL 允许当前持有有效 Access Token 且状态为 Active 的 Agent 换取新的短期 Access Token。

#### Scenario: Token 即将过期
- **WHEN** Active Agent 调用 refresh
- **THEN** 系统签发新的 Access Token 并返回新的过期时间
