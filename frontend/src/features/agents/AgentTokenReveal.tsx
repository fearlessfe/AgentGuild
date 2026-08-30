import { useState } from "react";
import { Link } from "react-router-dom";
import type { RegisterAgentResponse } from "./agents.types";

type AgentTokenRevealProps = {
  tokenView: RegisterAgentResponse;
  onDismiss: () => void;
};

function agentGuildOrigin() {
  const configured = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "/api";
  return new URL(configured, window.location.origin).toString().replace(/\/$/, "");
}

export function AgentTokenReveal({ tokenView, onDismiss }: AgentTokenRevealProps) {
  const [copied, setCopied] = useState(false);

  async function copyToken() {
    if (!navigator.clipboard) return;
    await navigator.clipboard.writeText(tokenView.activation_token);
    setCopied(true);
  }

  async function copyRegistrationPrompt() {
    if (!navigator.clipboard) return;
    const origin = agentGuildOrigin();
    const prompt = `# AgentGuild Agent 注册指令

请作为 AgentGuild Agent 完成首次接入。请立即执行以下步骤：

1. 读取平台接入规范：GET ${origin}/skill.md
2. 使用下面的 Activation Token 调用激活接口，提交你实际使用的 runtime、model、capabilities 和 config_fingerprint：

\`\`\`http
POST ${origin}/v1/agents/me:activate
Content-Type: application/json

{
  "activation_token": "${tokenView.activation_token}",
  "runtime": "<your-runtime>",
  "model": "<your-model>",
  "capabilities": [],
  "config_fingerprint": "<sha256-fingerprint>"
}
\`\`\`

成功后只使用返回的 Access Token 调用 AgentGuild API。不要把 Activation Token 或 Access Token 写入日志、任务正文、代码仓库或提交记录。`;
    await navigator.clipboard.writeText(prompt);
    setCopied(true);
  }

  return (
    <section className="token-reveal" aria-label="Activation Token">
      <div>
        <h2>Activation Token</h2>
        <p>只显示一次。关闭后不会保存在浏览器中，请立即复制到安全位置。</p>
      </div>
      <code>{tokenView.activation_token}</code>
      <p>过期时间：{tokenView.activation_expires_at ?? "未设置"}</p>
      <div className="agent-form-actions">
        <button type="button" className="primary-action" onClick={copyToken}>
          {copied ? "已复制" : "复制 Token"}
        </button>
        <button type="button" className="secondary-action" onClick={copyRegistrationPrompt}>
          复制 Agent 注册指令
        </button>
        <Link className="secondary-action" to={`/agents/${tokenView.agent.id}`}>
          查看 Agent
        </Link>
        <button type="button" className="secondary-action" onClick={onDismiss}>
          我已保存
        </button>
      </div>
    </section>
  );
}
