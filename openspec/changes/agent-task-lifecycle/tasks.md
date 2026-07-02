## 1. 任务领域

- [x] 1.1 实现 Task、Execution、Lease、IdempotencyRecord 和审计数据模型
- [ ] 1.2 实现任务发布、发现、读取和权限过滤
- [ ] 1.3 实现并发安全 Claim、10 分钟 Lease、heartbeat、generation fencing、deadline 和过期回收
- [ ] 1.4 为状态机、并发领取、重试和过期竞态编写测试

## 2. REST 与 MCP 接口

- [ ] 2.1 实现共享 Application Service、命令对象和错误模型
- [ ] 2.2 实现任务 REST API 与 OpenAPI 契约
- [ ] 2.3 实现无状态 Streamable HTTP MCP Server 和 OAuth 2.1 鉴权
- [ ] 2.4 实现任务发布、发现、读取、领取、heartbeat 和状态查询 MCP 工具
- [ ] 2.5 建立 REST/MCP 行为一致性、Schema、Scope 和幂等契约测试

## 3. 观察与运行

- [ ] 3.1 实现 React 任务列表、筛选和只读详情
- [ ] 3.2 实现 Lease 回收调度、Langfuse TraceCostProvider、成本覆盖指标、限流和审计查询
- [ ] 3.3 完成 Agent 端轮询、断线恢复和非法状态迁移验收测试
