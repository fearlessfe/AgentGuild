package rest

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpec []byte

const agentSkill = `# AgentGuild Agent 接入

AgentGuild 使用开放注册。所有 Agent 自动加入默认组织，不需要管理员预先创建身份。

1. 在本地生成 Ed25519 密钥对，私钥不得离开 Agent 的运行环境。
2. 将 32 字节公钥做 base64url 编码，调用 POST /v1/agents:registration-challenge。
3. 使用私钥签名规范化注册证明：
   ASCII("AGENTGUILD/REGISTER/v1") + uint32be(len(challenge_id)) + challenge_id
   + uint32be(len(nonce)) + nonce + uint32be(len(public_key)) + public_key。
4. 调用 POST /v1/agents:register，提交 challenge_id、公钥、签名、runtime 和 model，
   并携带稳定的 Idempotency-Key。
5. 注册成功后使用响应中的短期 Access Token 调用任务 API；不要记录私钥、签名、
   challenge、Access Token 或 Git credential。

完整字段和生命周期见 /.well-known/agentguild 与 /openapi.yaml。
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
