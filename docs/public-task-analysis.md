# Public Task Analysis

公开 GitHub Issue 任务默认使用 deterministic projection，因此即使没有模型凭证也会保持可领取。需要让平台分析源码时，在服务环境设置：

```dotenv
PUBLIC_TASK_ANALYZER=anthropic
ANTHROPIC_API_KEY=<secret>
ANTHROPIC_MODEL=claude-3-5-sonnet-20241022
```

也支持 OpenAI-compatible Chat Completions 服务。此模式读取通用环境变量，`BASE_URL` 可以是服务根地址或包含 `/v1` 的地址：

```dotenv
PUBLIC_TASK_ANALYZER=openai
BASE_URL=https://api.openai.com/v1
MODEL=gpt-5.6-sol
OPENAI_API_KEY=<secret>
```

worker 会先从 GitHub API 固定仓库默认分支 commit，再在临时目录中 shallow fetch 该 commit。分析器只收到受限的文本快照：跳过 `.git`、依赖/构建目录、二进制和明显凭证文件，并受文件数、单文件大小和总字节数限制。不会执行仓库中的脚本，也不会把快照写入数据库。

Claude API 返回的 JSON 必须包含问题诊断、影响、方案、实施步骤和验收条件。平台会重新做字段、数量和长度校验；分析请求超时、clone 失败、模型返回非法 JSON 或校验失败时，worker 回退 deterministic projection，保证任务仍能进入 claim 流程。

API key 只从环境变量读取并保留在进程内存中，不写入日志、任务正文、projection 或 Git 仓库。生产环境应使用 HTTPS 的 Anthropic endpoint，并限制出站网络到 GitHub 与模型服务。
