import { useState } from "react";
import { Card, Button, ApiNote } from "../../ui";

export function LoginPage() {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  const handleOIDCLogin = () => {
    if (import.meta.env.VITE_DEMO_MODE === "true") {
      window.location.href = "/agents";
      return;
    }
    window.location.href = "/oauth/oidc/login";
  };

  const handleLocalLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      const res = await fetch("/oauth/local/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
        credentials: "same-origin",
      });
      if (!res.ok) {
        setError("密码错误或未启用本地登录");
        return;
      }
      window.location.href = "/agents";
    } catch {
      setError("登录请求失败");
    }
  };

  return (
    <div className="auth-screen">
      <div className="auth-brand">
        <span className="rail-brand" aria-hidden="true">
          AG
        </span>
        <div>
          <div className="auth-title">AgentGuild</div>
          <div className="auth-tagline">企业 Agent 任务治理平台</div>
        </div>
      </div>
      <div className="auth-card">
        <Card title="登录" sub="使用企业身份登录以进入治理工作台">
          <form className="stack" onSubmit={handleLocalLogin}>
            <Button type="button" variant="primary" block lg icon="⛨" onClick={handleOIDCLogin}>
              使用企业 OIDC 登录
            </Button>
            <div className="auth-divider">
              <span>本地开发登录</span>
            </div>
            <div className="field">
              <label className="field-label" htmlFor="local-password">
                密码
              </label>
              <input
                id="local-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Local admin password"
              />
            </div>
            {error ? (
              <div className="auth-error" role="alert">
                {error}
              </div>
            ) : null}
            <Button type="submit" block>
              本地登录
            </Button>
            <p className="field-hint">本地登录仅在服务端启用时显示。</p>
          </form>
        </Card>
        <ApiNote status="available">OIDC 登录/回调与 local login 路由均存在；local login 仍需补入 OpenAPI。</ApiNote>
      </div>
    </div>
  );
}
