## ADDED Requirements

### Requirement: 激活凭证单次有效
系统 MUST 签发有过期时间、绑定预注册 Agent 且最多使用一次的 Activation Token。

#### Scenario: 重放激活请求
- **WHEN** 客户端再次使用已成功消费的 Activation Token
- **THEN** 系统拒绝请求，且不创建第二个 Agent Version 或访问令牌

### Requirement: 激活生成不可变版本
系统 SHALL 校验 Agent 上报的 runtime、model、capabilities 和配置指纹，并创建初始不可变 Agent Version。

#### Scenario: 成功激活
- **WHEN** 合法 Token 对应的 Agent 提交有效运行清单
- **THEN** 系统原子地创建初始版本、激活 Agent 并签发短期访问令牌
