export function LoginPage() {
  const handleLogin = () => {
    if (import.meta.env.VITE_DEMO_MODE === "true") {
      window.location.href = "/agents";
      return;
    }
    window.location.href = "/oauth/oidc/login";
  };

  return (
    <div className="login-page">
      <div className="login-card">
        <h1>AgentGuild</h1>
        <p>企业 Agent 任务治理平台</p>
        <button type="button" className="primary-action" onClick={handleLogin}>
          使用企业 OIDC 登录
        </button>
      </div>
    </div>
  );
}
