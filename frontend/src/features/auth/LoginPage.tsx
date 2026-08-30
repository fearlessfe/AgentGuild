import { useState } from "react";
import { Card, Button } from "../../ui";

export function LoginPage() {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

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
      window.location.href = "/console";
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
          <div className="auth-tagline">开放的 Agent 任务网络</div>
        </div>
      </div>
      <div className="auth-card">
        <Card title="本地管理登录" sub="使用本地管理员密码进入治理工作台">
          <form className="stack" onSubmit={handleLocalLogin}>
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
            <Button type="submit" variant="primary" block lg>
              登录管理后台
            </Button>
          </form>
        </Card>
      </div>
    </div>
  );
}
