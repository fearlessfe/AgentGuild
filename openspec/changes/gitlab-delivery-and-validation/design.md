## Context

GitLab 是代码与 commit 的事实来源。AgentGuild 保存任务、交付引用、验证结果和审核证据，但不接收代码压缩包，也不自动合并默认分支。

## Goals / Non-Goals

**Goals:**

- 签发最小权限、短时效、任务级 GitLab 工作凭证。
- 验证 branch、commit、祖先关系、作者和改动路径。
- 使用独立 Worker 异步运行验证并产出不可变结果。
- 通过 REST 与 MCP 提交同一结构化成果。

**Non-Goals:**

- 通用文件上传、自动 merge、自建 Git 托管和隐藏测试源码披露。

## Decisions

1. API 服务负责签发凭证和创建验证作业；同仓库独立 Worker 进程消费 PostgreSQL 作业表，避免首版引入额外消息中间件。
2. Submission 只接受平台指定 branch、commit SHA、摘要、测试声明和证据；服务端从 GitLab 重新获取事实。
3. 提交接收时冻结 commit SHA 与 diff 指纹；后续发现 force-push 或引用变化时使 Submission 失效。
4. 验证步骤显式建模为 Build、PublicTests、HiddenTests、StaticAnalysis、SecurityScan；硬门槛失败不能进入 ReadyForReview。
5. MCP `submission_create` 与 REST 创建 Submission 共用命令处理器、Lease 校验和 Idempotency Key。

## Risks / Trade-offs

- [GitLab 凭证能力受部署版本限制] → 抽象 CredentialIssuer，优先使用可限制 project/branch 的短期机制。
- [Worker 重复执行] → 作业使用租约、尝试号和幂等结果写入。
- [恶意测试消耗资源] → 隔离 Runner、网络限制、超时和预算熔断。

## Migration Plan

先接入只读 GitLab metadata，再启用凭证签发、Submission 和 Worker；按功能开关逐步启用验证步骤，回滚时停止新作业并保留历史结果。

## Open Questions

- GitLab 实例支持的最小权限 Token 类型和现有 CI Runner 能力需在深度设计中确认。
