import { ExternalLink, Plus, Trash2 } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import * as client from "../../api/client";
import type { RepositoryInventoryItem, RepositoryOnboardingSummary, RepositorySourceType } from "../../api/client";
import { Button, Card, DenseTable, EmptyState, PageHeader, StatusChip, type DenseRow } from "../../ui";

const SOURCE_LABELS: Record<RepositorySourceType, string> = {
  github_app: "GitHub App",
  public_github: "公开仓库",
};

export function RepositoryOnboardingScreen() {
  const [summary, setSummary] = useState<RepositoryOnboardingSummary | null>(null);
  const [publicRepo, setPublicRepo] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingRepo, setPendingRepo] = useState<string | null>(null);

  useEffect(() => {
    loadSummary();
  }, []);

  const loadSummary = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await client.getRepositoryOnboarding();
      setSummary(response.data);
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载仓库接入信息失败");
    } finally {
      setLoading(false);
    }
  };

  const onboarded = summary?.onboarded_repositories.items ?? [];
  const appRepositories = summary?.app_repositories.items ?? [];
  const appRepositoriesError = summary?.app_repositories_error;
  const showAppRepositoriesEmpty =
    !loading && summary?.github_app.configured && !appRepositoriesError && appRepositories.length === 0;
  const onboardedKeys = useMemo(() => new Set(onboarded.map(repositoryInventoryKey)), [onboarded]);

  const handleAddAppRepository = async (repo: string) => {
    try {
      setPendingRepo(repositoryKey("github_app", repo));
      const response = await client.addGitHubAppRepository(repo);
      setSummary((current) => appendOnboardedRepository(current, response.data));
    } catch (err) {
      setError(err instanceof Error ? err.message : "添加 GitHub App 仓库失败");
    } finally {
      setPendingRepo(null);
    }
  };

  const handleAddPublicRepository = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const repo = publicRepo.trim();
    if (!repo) return;
    try {
      setPendingRepo(repositoryKey("public_github", repo));
      const response = await client.addPublicRepository(repo);
      setSummary((current) => appendOnboardedRepository(current, response.data));
      setPublicRepo("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "添加公开仓库失败");
    } finally {
      setPendingRepo(null);
    }
  };

  const handleRemove = async (repo: RepositoryInventoryItem) => {
    if (!repo.id) return;
    try {
      setPendingRepo(repo.id);
      await client.removeRepository(repo.id);
      setSummary((current) =>
        current
          ? {
              ...current,
              onboarded_repositories: {
                items: current.onboarded_repositories.items.filter((item) => item.id !== repo.id),
              },
            }
          : current,
      );
    } catch (err) {
      setError(err instanceof Error ? err.message : "移除仓库失败");
    } finally {
      setPendingRepo(null);
    }
  };

  const candidateRows: DenseRow[] = appRepositories.map((repo) => {
    const candidateKey = repositoryKey("github_app", repo.full_name);
    const alreadyAdded = onboardedKeys.has(candidateKey);
    return {
      key: candidateKey,
      cells: [
        repo.full_name,
        <code>{repo.default_branch}</code>,
        repo.visibility,
        <Button
          icon={<Plus size={14} strokeWidth={1.8} />}
          variant={alreadyAdded ? "ghost" : "default"}
          disabled={alreadyAdded || pendingRepo === candidateKey}
          onClick={() => handleAddAppRepository(repo.full_name)}
        >
          {alreadyAdded ? "已添加" : "添加"}
        </Button>,
      ],
    };
  });

  const onboardedRows: DenseRow[] = onboarded.map((repo) => ({
    key: repo.id ?? repo.full_name,
    cells: [
      repo.full_name,
      <StatusChip tone={repo.source_type === "public_github" ? "info" : "success"}>
        {sourceLabel(repo.source_type)}
      </StatusChip>,
      <code>{repo.default_branch}</code>,
      repo.visibility,
      <Button
        icon={<Trash2 size={14} strokeWidth={1.8} />}
        variant="ghost"
        disabled={!repo.id || pendingRepo === repo.id}
        onClick={() => handleRemove(repo)}
      >
        移除
      </Button>,
    ],
  }));

  return (
    <div className="stack">
      <PageHeader title="仓库接入" sub="先确认 GitHub App 访问，再把仓库加入平台治理池。" />

      {error ? (
        <Card title="错误">
          <p role="alert" className="text-sm">
            {error}
          </p>
        </Card>
      ) : null}

      <div className="split-2">
        <div className="col">
          <Card
            title="Step 1 · GitHub App"
            sub={summary?.github_app.configured ? "已连接，可选择安装仓库" : "未连接，先完成 GitHub App 接入"}
          >
            {loading ? (
              <p className="text-sm">正在加载 GitHub App 状态...</p>
            ) : (
              <div className="stack-sm">
                <div className="row-between">
                  <span className="card-sub">状态</span>
                  <StatusChip tone={summary?.github_app.configured ? "success" : "neutral"}>
                    {summary?.github_app.configured ? "已配置" : "未配置"}
                  </StatusChip>
                </div>
                {summary?.github_app.configured ? (
                  <>
                    <div className="field">
                      <span className="field-label">App</span>
                      <div className="input">
                        <span>{summary.github_app.app_slug ?? `App ${summary.github_app.app_id ?? ""}`}</span>
                      </div>
                    </div>
                    <p className="field-hint">私钥材料只保存在服务端，不会回传到前端。</p>
                  </>
                ) : (
                  <p className="card-sub">请先在 Git 接入页安装或更新 GitHub App。</p>
                )}
                <div className="row">
                  <Button icon={<ExternalLink size={14} strokeWidth={1.8} />} onClick={() => (window.location.href = "/git-integration")}>
                    打开 Git 接入
                  </Button>
                </div>
              </div>
            )}
          </Card>

          <Card
            title="GitHub App 可见仓库"
            sub={`${appRepositories.length} 个候选仓库`}
            pad={Boolean(appRepositoriesError || showAppRepositoriesEmpty)}
          >
            {appRepositoriesError ? (
              <div role="alert" className="stack-sm">
                <p className="text-sm">无法读取 GitHub App 可见仓库。</p>
                <p className="card-sub">{appRepositoriesError}</p>
              </div>
            ) : showAppRepositoriesEmpty ? (
              <EmptyState title="暂无 GitHub App 可见仓库">
                <p className="card-sub">请确认 GitHub App 已安装到至少一个仓库。</p>
              </EmptyState>
            ) : (
              <DenseTable
                columns={["仓库", "默认分支", "可见性", "操作"]}
                rows={candidateRows}
                caption="GitHub App 候选仓库列表"
              />
            )}
          </Card>
        </div>

        <div className="col col--fill">
          <Card title="Step 2 · 添加仓库" sub="从 App 候选仓库或公开 GitHub 地址添加。">
            <form className="stack-sm" onSubmit={handleAddPublicRepository}>
              <label className="field" htmlFor="public-repo">
                <span className="field-label">公共仓库 URL 或 owner/repo</span>
                <input
                  id="public-repo"
                  aria-label="公共仓库 URL 或 owner/repo"
                  value={publicRepo}
                  onChange={(event) => setPublicRepo(event.target.value)}
                  placeholder="https://github.com/owner/repo"
                  autoComplete="off"
                />
                <span className="field-hint">添加仓库访问本身，不会创建 Issue 同步规则。</span>
              </label>
              <div className="row">
                <Button
                  type="submit"
                  icon={<Plus size={14} strokeWidth={1.8} />}
                  disabled={!publicRepo.trim() || pendingRepo === repositoryKey("public_github", publicRepo.trim())}
                >
                  添加公开仓库
                </Button>
              </div>
            </form>
          </Card>

          <Card title="已接入仓库" sub={`${onboarded.length} 个仓库`} pad={false}>
            <DenseTable
              columns={["仓库", "来源", "默认分支", "可见性", "操作"]}
              rows={onboardedRows}
              caption="已接入仓库列表"
            />
          </Card>
        </div>
      </div>
    </div>
  );
}

function sourceLabel(sourceType: RepositoryInventoryItem["source_type"]): string {
  return sourceType ? SOURCE_LABELS[sourceType] : "未知来源";
}

function repositoryKey(sourceType: RepositoryInventoryItem["source_type"], fullName: string): string {
  return `${sourceType ?? "unknown"}:${fullName}`;
}

function repositoryInventoryKey(repo: RepositoryInventoryItem): string {
  return repositoryKey(repo.source_type, repo.full_name);
}

function appendOnboardedRepository(
  current: RepositoryOnboardingSummary | null,
  repo: RepositoryInventoryItem,
): RepositoryOnboardingSummary | null {
  if (!current) return current;
  const existing = current.onboarded_repositories.items.filter(
    (item) => item.id !== repo.id && repositoryInventoryKey(item) !== repositoryInventoryKey(repo),
  );
  return {
    ...current,
    onboarded_repositories: { items: [repo, ...existing] },
  };
}
