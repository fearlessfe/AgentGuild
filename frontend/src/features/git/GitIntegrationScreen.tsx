import { ExternalLink, GitBranch, GitMerge, RefreshCcw } from "lucide-react";
import { useState, useEffect } from "react";
import { PageHeader, Card, Button, ProviderCard, StatusChip, ApiNote } from "../../ui";
import { getGitHubApp, testGitHubApp, deleteGitHubApp, githubManifestUrl, githubInstallUrl, type GitHubAppView } from "../../api/client";

const GITHUB_PERMISSIONS = ["Contents — 只读", "Issues — 读写", "Checks — 只读", "Metadata — 只读"];

export function GitIntegrationScreen() {
  const [githubApp, setGithubApp] = useState<GitHubAppView | null>(null);
  const [loading, setLoading] = useState(true);
  const [testResult, setTestResult] = useState<{ ok: boolean; repo_count?: number; error?: string } | null>(null);
  const [testing, setTesting] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const loadGitHubApp = async () => {
    try {
      setLoading(true);
      const response = await getGitHubApp();
      setGithubApp(response.data);
    } catch (error) {
      console.error("Failed to load GitHub App:", error);
      setGithubApp(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadGitHubApp();
  }, []);

  const handleConnect = () => {
    window.location.href = githubManifestUrl();
  };

  const handleInstall = () => {
    window.location.href = githubInstallUrl();
  };

  const handleTest = async () => {
    try {
      setTesting(true);
      setTestResult(null);
      const response = await testGitHubApp();
      setTestResult(response.data);
    } catch (error) {
      setTestResult({ ok: false, error: String(error) });
    } finally {
      setTesting(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm("确定要删除 GitHub App 配置吗？此操作不可撤销。")) {
      return;
    }
    try {
      setDeleting(true);
      await deleteGitHubApp();
      await loadGitHubApp();
      setTestResult(null);
    } catch (error) {
      console.error("Failed to delete GitHub App:", error);
      alert("删除失败：" + String(error));
    } finally {
      setDeleting(false);
    }
  };

  const isConfigured = githubApp?.configured ?? false;
  const isInstalled = Boolean(githubApp?.installation_id);
  const canInstall = isConfigured && !isInstalled && Boolean(githubApp?.app_slug);

  return (
    <div className="stack">
      <PageHeader title="Git 接入" sub="使用通用 Git 提供商外壳；当前仅启用 GitHub。" />
      <div className="split-2">
        <div className="col">
          {loading ? (
            <Card>
              <p>加载中...</p>
            </Card>
          ) : isConfigured ? (
            <ProviderCard
              logo={<GitBranch size={18} strokeWidth={1.8} />}
              name="GitHub"
              note={githubApp?.app_slug ? `${isInstalled ? "已安装" : "待安装"} · ${githubApp.app_slug}` : "已创建"}
              permissions={GITHUB_PERMISSIONS}
            >
              <div className="stack-sm">
                <div className="row-between">
                  <span className="card-sub">状态</span>
                  <StatusChip tone={isInstalled ? "success" : "warning"}>
                    {isInstalled ? "已安装" : "已创建，待安装"}
                  </StatusChip>
                </div>
                <div className="field">
                  <label className="field-label">App ID</label>
                  <div className="input">
                    <span>{githubApp?.app_id}</span>
                  </div>
                </div>
                <div className="field">
                  <label className="field-label">Installation ID</label>
                  <div className="input">
                    <span>{isInstalled ? githubApp?.installation_id : "待安装"}</span>
                  </div>
                </div>
                <div className="field">
                  <label className="field-label">GitHub App 私钥</label>
                  <div className="input">
                    <span className="faint">已保存 · 不可读取</span>
                  </div>
                  <p className="field-hint">出于安全，已存储的私钥永不回显。</p>
                </div>
                {testResult && (
                  <div className="field">
                    <label className="field-label">连接检测结果</label>
                    <div className="input">
                      {testResult.ok ? (
                        <span style={{ color: "green" }}>✓ 连接成功 · 可访问 {testResult.repo_count} 个仓库</span>
                      ) : (
                        <span style={{ color: "red" }}>✗ 连接失败 · {testResult.error}</span>
                      )}
                    </div>
                  </div>
                )}
                <div className="row">
                  {isInstalled ? (
                    <Button icon={<RefreshCcw size={14} strokeWidth={1.8} />} onClick={handleTest} disabled={testing}>
                      {testing ? "检测中..." : "检测连接"}
                    </Button>
                  ) : canInstall ? (
                    <Button icon={<ExternalLink size={14} strokeWidth={1.8} />} onClick={handleInstall}>
                      安装 GitHub App
                    </Button>
                  ) : (
                    <Button icon={<ExternalLink size={14} strokeWidth={1.8} />} onClick={handleConnect}>
                      重新连接 GitHub
                    </Button>
                  )}
                  <Button variant="danger" onClick={handleDelete} disabled={deleting}>
                    {deleting ? "删除中..." : "删除"}
                  </Button>
                </div>
              </div>
            </ProviderCard>
          ) : (
            <ProviderCard
              logo={<GitBranch size={18} strokeWidth={1.8} />}
              name="GitHub"
              note="未配置"
              permissions={GITHUB_PERMISSIONS}
            >
              <div className="stack-sm">
                <div className="row-between">
                  <span className="card-sub">状态</span>
                  <StatusChip tone="neutral">未配置</StatusChip>
                </div>
                <p className="card-sub">点击下方按钮开始配置 GitHub App</p>
                <div className="row">
                  <Button onClick={handleConnect}>连接 GitHub</Button>
                </div>
              </div>
            </ProviderCard>
          )}
        </div>
        <div className="col">
          <ProviderCard logo={<GitMerge size={18} strokeWidth={1.8} />} name="GitLab" note="即将支持" disabled>
            <div className="row-between">
              <span className="card-sub">状态</span>
              <StatusChip tone="neutral">即将支持</StatusChip>
            </div>
          </ProviderCard>
          <ApiNote status="available">GET/POST/DELETE /v1/github-app 已提供 CRUD；前端已接入。</ApiNote>
        </div>
      </div>
    </div>
  );
}
