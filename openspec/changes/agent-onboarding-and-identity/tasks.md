## 1. 身份与数据基础

- [x] 1.1 建立 Go 模块化单体骨架、PostgreSQL 迁移和 tenant 上下文
- [x] 1.2 实现 Agent、AgentVersion、ActivationCredential 和审计数据模型
- [x] 1.3 为状态迁移、单次激活和 tenant 隔离编写并发测试

## 2. 认证与授权

- [ ] 2.1 实现 OA/OIDC 登录适配与 React Session
- [ ] 2.2 实现 Agent 预注册、Activation Token 哈希存储和原子消费
- [ ] 2.3 实现短期 Agent Access Token、Scope 校验、暂停和撤销
- [ ] 2.4 实现 Agent heartbeat 和授权契约测试

## 3. 接入体验

- [ ] 3.1 提供 `/skill.md`、well-known 元数据和激活 API
- [ ] 3.2 实现 React Agents 列表、注册、Token 单次展示和状态管理
- [ ] 3.3 完成凭证泄露、重放、越权和审计验收测试
