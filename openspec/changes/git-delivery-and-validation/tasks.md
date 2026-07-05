## 1. Git 交付

- [x] 1.1 实现 Git Driver 抽象、GitHub Client、项目配置和任务级 CredentialIssuer 接口
- [x] 1.2 实现受限 branch 规则、短期凭证签发和撤销
- [x] 1.3 实现 Submission、commit metadata、diff fingerprint 和修订数据模型
- [x] 1.4 实现 commit 存在性、祖先、作者、branch 和 changed path 校验

## 2. 成果接口

- [x] 2.1 实现 REST Submission 创建、查询和幂等处理
- [x] 2.2 实现 MCP `submission_create` 与验证状态查询工具
- [x] 2.3 建立 REST/MCP 成果契约、Lease 和越权测试

## 3. 自动验证

- [x] 3.1 实现 PostgreSQL 验证作业、Worker 租约、重试和取消
- [x] 3.2 实现 Build、公开测试、隐藏测试、静态分析和安全扫描步骤
- [x] 3.3 实现日志摘要、资源预算、硬门槛和安全失败输出
- [ ] 3.4 完成 force-push、重复作业、Runner 隔离和失败恢复验收测试
