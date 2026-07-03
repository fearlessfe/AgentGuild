import { useState } from "react";
import { Link } from "react-router-dom";
import type { RegisterAgentResponse } from "./agents.types";

type AgentTokenRevealProps = {
  tokenView: RegisterAgentResponse;
  onDismiss: () => void;
};

export function AgentTokenReveal({ tokenView, onDismiss }: AgentTokenRevealProps) {
  const [copied, setCopied] = useState(false);

  async function copyToken() {
    if (!navigator.clipboard) return;
    await navigator.clipboard.writeText(tokenView.activation_token);
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
