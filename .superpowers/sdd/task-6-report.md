# Task 6 Report

## Scope

实现 Agent 接入体验的缺失部分，限定在以下文件范围内：

- `skill.md`
- `backend/internal/transport/rest/well_known.go`
- `backend/internal/transport/rest/well_known_test.go`
- `backend/internal/transport/rest/openapi.yaml`
- `backend/internal/transport/rest/router.go`

未读取、修改、暂存或回滚 `openspec/changes/gitlab-delivery-and-validation/**` 及其他禁止范围文件。

## TDD Record

### RED

先新增了 `backend/internal/transport/rest/well_known_test.go`，覆盖三项缺失行为：

- `TestWellKnownExposesActivationMetadata`
- `TestWellKnownDocumentedInOpenAPI`
- `TestSkillGuideIncludesActivationFlow`

RED 失败命令与摘要：

```bash
cd backend && go test ./internal/transport/rest -run TestWellKnown -count=1
```

失败摘要：

- `GET /.well-known/agentguild` 返回 `404`，期望 `200`
- `openapi.yaml` 不包含 `/.well-known/agentguild`

在第一次最小实现后又进行了第二轮 TDD：

```bash
cd backend && go test ./internal/transport/rest -run TestSkill -count=1
```

失败摘要：

- `skill.md` 已存在但未命中测试要求的安全提示短语 `不要记录 Activation Token`

### GREEN

实现后通过以下验证：

```bash
cd backend && go test ./internal/transport/rest -run "TestWellKnown|TestSkill" -count=1
cd backend && go test ./internal/transport/rest -count=1
git diff --check
```

结果：

- 所有目标测试通过
- REST 包全量测试通过
- `git diff --check` 无输出

## Implementation Summary

### 1. Well-known metadata endpoint

- 新增 `backend/internal/transport/rest/well_known.go`
- 提供 `GET /.well-known/agentguild`
- 返回固定 JSON：
  - `version`
  - `activation_url`
  - `refresh_url`
  - `heartbeat_url`
  - `scopes`

### 2. Router mount

- 在 `backend/internal/transport/rest/router.go` 根路由挂载 `/.well-known/agentguild`

### 3. OpenAPI updates

- 在 `backend/internal/transport/rest/openapi.yaml` 增加 `GET /.well-known/agentguild`
- 新增 `AgentWellKnown` schema
- 为 `POST /v1/agents/me:activate` 增加描述与请求/响应示例
- 为 `POST /v1/agents/me:refresh` 增加描述与响应示例

### 4. Agent skill guide

- 在仓库根创建 `skill.md`
- 包含：
  - Activation Token 获取方式
  - `POST /v1/agents/me:activate` 激活说明
  - `Access Token` 调用 `tasks:*` API 说明
  - 15 分钟有效期与 `POST /v1/agents/me:refresh` 续期说明
  - 不要记录 Activation Token 的安全提示

## Files Changed

- `skill.md`
- `backend/internal/transport/rest/well_known.go`
- `backend/internal/transport/rest/well_known_test.go`
- `backend/internal/transport/rest/openapi.yaml`
- `backend/internal/transport/rest/router.go`
- `.superpowers/sdd/task-6-report.md`

## Concerns

无额外功能性顾虑。实现使用固定公开 URL `https://api.agentguild.dev/...`，与任务 brief 给出的 contract 保持一致。

## Task 6 Fix Record

### Reviewer issue

- `backend/internal/transport/rest/well_known_test.go` 原有 OpenAPI 断言只在全文件范围检查字符串包含，不能证明 `/v1/agents/me:activate` path 节点自身仍保留了激活请求与 access-token 响应示例。

### RED

先将 `TestWellKnownDocumentedInOpenAPI` 收紧为 path-section 级别断言：

- 截取 `/.well-known/agentguild` section，保留 well-known path 自身存在性和 schema 引用检查
- 截取 `/v1/agents/me:activate` section，要求该 section 内同时包含：
  - `ActivateAgentRequest` request schema
  - request example
  - access-token response example

为证明新测试能抓住 reviewer 提到的缺口，临时从工作树中的 `backend/internal/transport/rest/openapi.yaml` 删除 activate path 下的 request/response examples 后运行：

```bash
cd backend && go test ./internal/transport/rest -run TestWellKnownDocumentedInOpenAPI -count=1
```

失败摘要：

- `TestWellKnownDocumentedInOpenAPI` 在 `/v1/agents/me:activate` section 内未找到 `examples:`
- 失败输出只展示 activate path section，证明断言已限定在目标 path 节点，而非全文件误命中

临时改动随后已还原，未纳入提交。

### GREEN

还原 `openapi.yaml` 后运行：

```bash
cd backend && go test ./internal/transport/rest -run 'TestWellKnown|TestSkill' -count=1
cd backend && go test ./internal/transport/rest -count=1
git diff --check
```

结果摘要：

- 目标测试通过
- `rest` 包全量测试通过
- `git diff --check` 无输出

### Fix summary

- 将 OpenAPI 文档测试改为按 path section 定向断言，避免 `AgentWellKnown` schema 或其他 section 中的同名字符串掩盖 `/v1/agents/me:activate` 自身缺失示例的问题
- 未修改生产代码，也未修改 `openapi.yaml`，因为现有文档字段完整，问题仅在测试覆盖力度不足
