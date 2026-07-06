import { useState } from "react";

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
    <div className="login-page">
      <div className="login-card">
        <h1>AgentGuild</h1>
        <p>企业 Agent 任务治理平台</p>
        <button type="button" className="primary-action" onClick={handleOIDCLogin}>
          使用企业 OIDC 登录
        </button>
        <hr />
        <form onSubmit={handleLocalLogin}>
          <label htmlFor="local-password">本地开发登录</label>
          <input
            id="local-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Local admin password"
          />
          {error ? <p className="error">{error}</p> : null}
          <button type="submit" className="secondary-action">
            本地登录
          </button>
        </form>
      </div>
    </div>
  );
}
