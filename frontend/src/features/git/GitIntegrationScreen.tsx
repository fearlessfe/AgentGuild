import { ExternalLink, GitBranch, GitMerge, Plus, RefreshCcw } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { ApiNote, Button, Card, PageHeader, ProviderCard, StatusChip } from "../../ui";
import * as client from "../../api/client";

const GITHUB_PERMISSIONS = ["Contents — 只读", "Issues — 读写", "Checks — 只读", "Metadata — 只读"];

type TestResult = { ok: boolean; repo_count?: number; error?: string };

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function GitIntegrationScreen() {
  const [githubApps, setGitHubApps] = useState<client.GitHubAppView[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [testResults, setTestResults] = useState<Record<string, TestResult>>({});
  const [actionErrors, setActionErrors] = useState<Record<string, string>>({});
  const [actionStatus, setActionStatus] = useState("");
  const [refreshError, setRefreshError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [testing, setTesting] = useState<Set<string>>(new Set());
  const [deleting, setDeleting] = useState<Set<string>>(new Set());
  const refreshEpoch = useRef(0);
  const confirmedDeleted = useRef(new Set<string>());
  const deleteKeys = useRef<Record<string, string>>({});

  useEffect(() => {
    let active = true;
    const load = async () => {
      const epoch = ++refreshEpoch.current;
      try {
        setLoading(true);
        setLoadError(null);
        const response = await client.listGitHubApps();
        if (active && epoch === refreshEpoch.current) setGitHubApps(withoutDeletedApps(response.data.items, confirmedDeleted.current));
      } catch (error) {
        if (active) setLoadError(errorMessage(error, "加载 GitHub App 失败"));
      } finally {
        if (active) setLoading(false);
      }
    };
    void load();
    return () => {
      active = false;
    };
  }, [loadAttempt]);

  const handleConnect = () => {
    window.location.href = client.githubManifestUrl();
  };

  const handleTest = async (app: client.GitHubAppView) => {
    setTesting((current) => new Set(current).add(app.id));
    setActionErrors((current) => withoutKey(current, app.id));
    setTestResults((current) => withoutKey(current, app.id));
    try {
      const response = await client.testGitHubApp(app.id);
      setTestResults((current) => ({ ...current, [app.id]: response.data }));
    } catch (error) {
      setTestResults((current) => ({
        ...current,
        [app.id]: { ok: false, error: errorMessage(error, "连接检测失败") },
      }));
    } finally {
      setTesting((current) => withoutSetItem(current, app.id));
    }
  };

  const handleDelete = async (app: client.GitHubAppView) => {
    const appName = githubAppName(app);
    if (!window.confirm(`确定要删除 GitHub App ${appName} 吗？此操作不可撤销。`)) return;
    setActionStatus("");
    setRefreshError(null);
    setDeleting((current) => new Set(current).add(app.id));
    setActionErrors((current) => withoutKey(current, app.id));
    deleteKeys.current[app.id] ??= client.createIdempotencyKey();
    try {
      await client.deleteGitHubApp(app.id, { idempotencyKey: deleteKeys.current[app.id] });
      delete deleteKeys.current[app.id];
      confirmedDeleted.current.add(app.id);
      setGitHubApps((current) => removeDeletedGitHubApp(current, app.id));
      setTestResults((current) => withoutKey(current, app.id));
      setActionStatus(`已删除 GitHub App ${appName}`);

      try {
        const epoch = ++refreshEpoch.current;
        const response = await client.listGitHubApps();
        if (epoch === refreshEpoch.current) setGitHubApps(withoutDeletedApps(response.data.items, confirmedDeleted.current));
      } catch (error) {
        setRefreshError(errorMessage(error, "刷新 GitHub App 列表失败"));
      }
    } catch (error) {
      if (error instanceof client.ApiError) delete deleteKeys.current[app.id];
      setActionErrors((current) => ({
        ...current,
        [app.id]: errorMessage(error, "删除 GitHub App 失败"),
      }));
    } finally {
      setDeleting((current) => withoutSetItem(current, app.id));
    }
  };

  const handleRefresh = async () => {
    const epoch = ++refreshEpoch.current;
    setRefreshing(true);
    setRefreshError(null);
    try {
      const response = await client.listGitHubApps();
      if (epoch === refreshEpoch.current) setGitHubApps(withoutDeletedApps(response.data.items, confirmedDeleted.current));
    } catch (error) {
      setRefreshError(errorMessage(error, "刷新 GitHub App 列表失败"));
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div className="stack">
      <PageHeader title="Git 接入" sub="使用通用 Git 提供商外壳；当前仅启用 GitHub。" />
      <div className="row">
        <Button icon={<Plus size={14} strokeWidth={1.8} />} onClick={handleConnect}>
          新增 GitHub App
        </Button>
      </div>

      {loadError ? (
        <Card title="GitHub App 加载失败">
          <div className="stack-sm">
            <p role="alert" className="text-sm">{loadError}</p>
            <div className="row">
              <Button aria-label="重试加载 GitHub App" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
                重试
              </Button>
            </div>
          </div>
        </Card>
      ) : null}
      {actionStatus ? (
        <p role="status" aria-label="GitHub App 操作结果" className="text-sm">
          {actionStatus}
        </p>
      ) : null}
      {refreshError ? (
        <div className="stack-sm">
          <p role="alert" className="text-sm">删除已成功，但刷新 GitHub App 列表失败：{refreshError}</p>
          <div className="row">
            <Button
              aria-label={refreshing ? "正在刷新 GitHub App 列表" : "重试刷新 GitHub App 列表"}
              aria-busy={refreshing}
              disabled={refreshing}
              onClick={() => void handleRefresh()}
            >
              {refreshing ? "正在刷新" : "重试刷新"}
            </Button>
          </div>
        </div>
      ) : null}

      <div className="split-2">
        <div className="col">
          {loading ? (
            <Card title="GitHub Apps">
              <p role="status">正在加载 GitHub App...</p>
            </Card>
          ) : loadError ? null : githubApps.length === 0 ? (
            <Card title="GitHub Apps">
              <p>尚未配置 GitHub App。使用“新增 GitHub App”开始接入。</p>
            </Card>
          ) : (
            <ul className="steps" aria-label="GitHub App 列表">
              {githubApps.map((app) => (
                <li key={app.id}>
                  <GitHubAppCard
                    app={app}
                    testResult={testResults[app.id]}
                    actionError={actionErrors[app.id]}
                    testing={testing.has(app.id)}
                    deleting={deleting.has(app.id)}
                    onTest={() => void handleTest(app)}
                    onDelete={() => void handleDelete(app)}
                    onInstall={() => {
                      window.location.href = client.githubInstallUrl(app.id);
                    }}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="col">
          <ProviderCard logo={<GitMerge size={18} strokeWidth={1.8} />} name={<h2>GitLab</h2>} note="即将支持" disabled>
            <div className="row-between">
              <span className="card-sub">状态</span>
              <StatusChip tone="neutral">即将支持</StatusChip>
            </div>
          </ProviderCard>
          <ApiNote status="available">GitHub App 多实例 API 已接入；每个 App 可独立安装、检测和删除。</ApiNote>
        </div>
      </div>
    </div>
  );
}

function GitHubAppCard({
  app,
  testResult,
  actionError,
  testing,
  deleting,
  onTest,
  onDelete,
  onInstall,
}: {
  app: client.GitHubAppView;
  testResult?: TestResult;
  actionError?: string;
  testing: boolean;
  deleting: boolean;
  onTest: () => void;
  onDelete: () => void;
  onInstall: () => void;
}) {
  const installed = Boolean(app.installation_id);
  const appName = githubAppName(app);
  const label = `${appName} · ${app.installation_account_login ?? "待选择安装账户"}`;

  return (
    <ProviderCard
      logo={<GitBranch size={18} strokeWidth={1.8} />}
      name={<h2>{label}</h2>}
      note={app.is_default ? "默认 GitHub App" : "GitHub App"}
      permissions={GITHUB_PERMISSIONS}
    >
      <div className="stack-sm">
        <div className="row-between">
          <span className="card-sub">状态</span>
          <StatusChip tone={installed ? "success" : "warning"}>
            {installed ? "已安装" : "已创建，待安装"}
          </StatusChip>
        </div>
        <div className="field">
          <span className="field-label">App ID</span>
          <div className="input"><span>{app.app_id}</span></div>
        </div>
        <div className="field">
          <span className="field-label">Installation ID</span>
          <div className="input"><span>{installed ? app.installation_id : "待安装"}</span></div>
        </div>
        <p className="field-hint">私钥已安全保存且永不回显。</p>

        {testResult ? (
          <div role="status" className="input">
            {testResult.ok
              ? `连接成功 · 可访问 ${testResult.repo_count ?? 0} 个仓库`
              : `连接失败 · ${testResult.error ?? "未知错误"}`}
          </div>
        ) : null}
        {actionError ? <p role="alert" className="text-sm">删除失败：{actionError}</p> : null}

        <div className="row">
          {installed ? (
            <Button
              icon={<RefreshCcw size={14} strokeWidth={1.8} />}
              onClick={onTest}
              disabled={testing || deleting}
              aria-label={testing ? `正在检测 ${appName}` : `检测 ${appName}`}
              aria-busy={testing}
            >
              {testing ? `正在检测 ${appName}` : "检测连接"}
            </Button>
          ) : (
            <Button
              icon={<ExternalLink size={14} strokeWidth={1.8} />}
              onClick={onInstall}
              disabled={deleting}
              aria-label={`安装 ${appName}`}
            >
              安装 GitHub App
            </Button>
          )}
          <Button
            variant="danger"
            onClick={onDelete}
            disabled={deleting || testing}
            aria-label={deleting ? `正在删除 ${appName}` : `删除 ${appName}`}
            aria-busy={deleting}
          >
            {deleting ? `正在删除 ${appName}` : "删除"}
          </Button>
        </div>
      </div>
    </ProviderCard>
  );
}

function withoutKey<T>(record: Record<string, T>, key: string): Record<string, T> {
  const next = { ...record };
  delete next[key];
  return next;
}

function withoutSetItem(items: Set<string>, item: string): Set<string> {
  const next = new Set(items);
  next.delete(item);
  return next;
}

function removeDeletedGitHubApp(apps: client.GitHubAppView[], deletedID: string): client.GitHubAppView[] {
  const deleted = apps.find((app) => app.id === deletedID);
  const remaining = apps.filter((app) => app.id !== deletedID);
  if (!deleted?.is_default || remaining.length === 0) return remaining;

  const nextDefault = remaining.reduce((earliest, app) => (
    compareGitHubApps(app, earliest) < 0 ? app : earliest
  ));
  return remaining.map((app) => ({ ...app, is_default: app.id === nextDefault.id }));
}

function compareGitHubApps(left: client.GitHubAppView, right: client.GitHubAppView): number {
  const createdAt = (left.created_at ?? "").localeCompare(right.created_at ?? "");
  return createdAt || left.id.localeCompare(right.id);
}

function githubAppName(app: client.GitHubAppView): string {
  return app.app_slug || `App ${app.app_id}`;
}

function withoutDeletedApps(apps: client.GitHubAppView[], deleted: Set<string>): client.GitHubAppView[] {
  return apps.filter((app) => !deleted.has(app.id));
}
