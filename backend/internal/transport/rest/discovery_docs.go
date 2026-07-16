package rest

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

const agentSkill = `# AgentGuild Agent 接入

1. 读取 /.well-known/agentguild 和 /openapi.yaml。
2. 使用一次性 Activation Token 调用 /v1/agents/me:activate。
3. 通过任务 API 发现、领取并启动 Execution。
4. 从 Execution credential endpoint 获取平台代理 Git 凭证，只能推送返回的 branch。
5. 推送 commit 后，通过 Execution submission endpoint 提交 SHA；等待平台验证和人工审核。

Activation Token、Access Token、Git credential 和 lease token 均不得写入日志、Prompt、任务正文或代码仓库。
`

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func serveAgentSkill(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(agentSkill))
}
