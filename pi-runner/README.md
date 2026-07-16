# AgentGuild Pi runner

该进程是 Issue 分析流水线的 Pi Agent 执行边界。它从 stdin 读取一个
`agentguild.pi-analysis.v1` JSON job，并只向 stdout 写入一个同版本 JSON result。

- Pi SDK 固定为 `earendil-works/pi` `v0.80.8`，upstream commit
  `fae7176cb9f7c4725a40d9d481d8d70b80f18086`。
- 每个 job 使用 `SessionManager.inMemory()`、内存 settings 和内存 credential store。
- 只启用 `read`、`grep`、`find`、`ls` 与 `submit_task_specification`。
- 只有终止型 `submit_task_specification` 工具的唯一一次合法调用能产生正式任务规格；
  stdout 文本和 transcript 永远不会被解析为任务字段。
- runner 自身不是安全沙箱。生产环境必须把整个进程放入固定 digest 的容器，使用
  只读 workspace、默认无外网、资源限制，并且不得挂载宿主 `.pi`、auth 或 session。

运行：

```bash
npm ci
npm run build
PI_MODEL_API_KEY=... npm start < job.json
```
