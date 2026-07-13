import { ExternalLink, Plus, Trash2 } from "lucide-react";
import { type FormEvent, useEffect, useMemo, useRef, useState } from "react";
import * as client from "../../api/client";
import type {
  GitHubAppView,
  Repository,
  RepositoryInventoryItem,
  RepositoryOnboardingSummary,
  RepositorySourceType,
} from "../../api/client";
import { Button, Card, DenseTable, PageHeader, SearchableSelect, StatusChip, type DenseRow } from "../../ui";

const SOURCE_LABELS: Record<RepositorySourceType, string> = {
  github_app: "GitHub App",
  public_github: "公开仓库",
};

type RepositorySourceMode = "github_app" | "public_github";
type RepositoryLoadState = { loading: boolean; error: string | null };

export function RepositoryOnboardingScreen() {
  const [apps, setApps] = useState<GitHubAppView[]>([]);
  const [summary, setSummary] = useState<RepositoryOnboardingSummary | null>(null);
  const [sourceMode, setSourceMode] = useState<RepositorySourceMode>("github_app");
  const [selectedAppID, setSelectedAppID] = useState("");
  const [repositoriesByApp, setRepositoriesByApp] = useState<Record<string, Repository[]>>({});
  const [repositoryLoadStates, setRepositoryLoadStates] = useState<Record<string, RepositoryLoadState>>({});
  const [repositoryQuery, setRepositoryQuery] = useState("");
  const [selectedRepository, setSelectedRepository] = useState<Repository | null>(null);
  const [publicRepo, setPublicRepo] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingRepo, setPendingRepo] = useState<string | null>(null);
  const requestVersions = useRef<Record<string, number>>({});

  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        setLoading(true);
        setError(null);
        const [appsResponse, onboardingResponse] = await Promise.all([
          client.listGitHubApps(),
          client.getRepositoryOnboarding(),
        ]);
        if (!active) return;
        setApps(appsResponse.data.items);
        setSummary(onboardingResponse.data);
      } catch (err) {
        if (active) setError(errorMessage(err, "加载仓库接入信息失败"));
      } finally {
        if (active) setLoading(false);
      }
    };
    void load();
    return () => {
      active = false;
    };
  }, []);

  const onboarded = summary?.onboarded_repositories?.items ?? [];
  const installedApps = useMemo(
    () => apps.filter((app) => Boolean(app.installation_id)),
    [apps],
  );
  const onboardedNames = useMemo(
    () => new Set(onboarded.map((repo) => repo.full_name.toLowerCase())),
    [onboarded],
  );
  const selectedAppRepositories = repositoriesByApp[selectedAppID] ?? [];
  const filteredRepositories = useMemo(() => {
    const normalizedQuery = repositoryQuery.trim().toLowerCase();
    return selectedAppRepositories.filter(
      (repo) =>
        !onboardedNames.has(repo.full_name.toLowerCase()) &&
        repo.full_name.toLowerCase().includes(normalizedQuery),
    );
  }, [onboardedNames, repositoryQuery, selectedAppRepositories]);
  const currentRepositoryLoad = repositoryLoadStates[selectedAppID];

  const loadRepositories = async (appID: string, force = false) => {
    if (
      !appID ||
      (!force && (Object.prototype.hasOwnProperty.call(repositoriesByApp, appID) || repositoryLoadStates[appID]?.loading))
    ) return;
    const version = (requestVersions.current[appID] ?? 0) + 1;
    requestVersions.current[appID] = version;
    setRepositoryLoadStates((current) => ({ ...current, [appID]: { loading: true, error: null } }));
    try {
      const response = await client.listGitHubAppRepositories(appID);
      if (requestVersions.current[appID] !== version) return;
      setRepositoriesByApp((current) => ({ ...current, [appID]: response.data.items }));
      setRepositoryLoadStates((current) => ({ ...current, [appID]: { loading: false, error: null } }));
    } catch (err) {
      if (requestVersions.current[appID] !== version) return;
      setRepositoryLoadStates((current) => ({
        ...current,
        [appID]: { loading: false, error: errorMessage(err, "加载授权仓库失败") },
      }));
    }
  };

  const clearRepositoryChoice = () => {
    setRepositoryQuery("");
    setSelectedRepository(null);
  };

  const handleSourceChange = (nextSource: RepositorySourceMode) => {
    setSourceMode(nextSource);
    setSelectedAppID("");
    setPublicRepo("");
    clearRepositoryChoice();
  };

  const handleAppChange = (appID: string) => {
    setSelectedAppID(appID);
    clearRepositoryChoice();
    if (appID) void loadRepositories(appID);
  };

  const handleAddAppRepository = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedAppID || !selectedRepository) return;
    const key = repositoryKey("github_app", selectedRepository.full_name);
    try {
      setError(null);
      setPendingRepo(key);
      const response = await client.addGitHubAppRepository(selectedAppID, selectedRepository.full_name);
      setSummary((current) => appendOnboardedRepository(current, response.data));
      clearRepositoryChoice();
    } catch (err) {
      setError(errorMessage(err, "添加 GitHub App 仓库失败"));
    } finally {
      setPendingRepo(null);
    }
  };

  const handleAddPublicRepository = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const repo = publicRepo.trim();
    if (!repo) return;
    try {
      setError(null);
      setPendingRepo(repositoryKey("public_github", repo));
      const response = await client.addPublicRepository(repo);
      setSummary((current) => appendOnboardedRepository(current, response.data));
      setPublicRepo("");
    } catch (err) {
      setError(errorMessage(err, "添加公开仓库失败"));
    } finally {
      setPendingRepo(null);
    }
  };

  const handleRemove = async (repo: RepositoryInventoryItem) => {
    if (!repo.id) return;
    try {
      setError(null);
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
      setError(errorMessage(err, "移除仓库失败"));
    } finally {
      setPendingRepo(null);
    }
  };

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
        onClick={() => void handleRemove(repo)}
      >
        移除
      </Button>,
    ],
  }));

  return (
    <div className="stack">
      <PageHeader title="仓库接入" sub="选择 GitHub App 授权仓库，或添加公开 GitHub 仓库。" />

      {loading ? <p role="status" className="text-sm">正在加载仓库接入信息...</p> : null}
      {error ? <p role="alert" className="form-error">{error}</p> : null}

      <div className="split-2">
        <div className="col">
          <Card title="添加仓库" sub="选择一种仓库来源并完成接入。">
            <fieldset className="source-selector" disabled={loading}>
              <legend className="field-label">仓库来源</legend>
              <label className="source-selector__option">
                <input
                  type="radio"
                  name="repository-source"
                  value="github_app"
                  checked={sourceMode === "github_app"}
                  onChange={() => handleSourceChange("github_app")}
                />
                <span>GitHub App 授权仓库</span>
              </label>
              <label className="source-selector__option">
                <input
                  type="radio"
                  name="repository-source"
                  value="public_github"
                  checked={sourceMode === "public_github"}
                  onChange={() => handleSourceChange("public_github")}
                />
                <span>公开仓库</span>
              </label>
            </fieldset>

            {sourceMode === "github_app" ? (
              <form className="stack-sm" onSubmit={handleAddAppRepository}>
                {!loading && installedApps.length === 0 ? (
                  <div className="stack-sm">
                    <p className="text-sm">暂无已安装的 GitHub App</p>
                    <p className="field-hint">请先完成 GitHub App 安装，再选择其授权仓库。</p>
                    <div className="row">
                      <Button icon={<ExternalLink size={14} strokeWidth={1.8} />} type="button" onClick={() => (window.location.href = "/git-integration")}>
                        打开 Git 接入
                      </Button>
                    </div>
                  </div>
                ) : (
                  <>
                    <label className="field" htmlFor="github-app-select">
                      <span className="field-label">GitHub App</span>
                      <select
                        id="github-app-select"
                        value={selectedAppID}
                        onChange={(event) => handleAppChange(event.target.value)}
                        disabled={loading}
                      >
                        <option value="">请选择 GitHub App</option>
                        {installedApps.map((app) => (
                          <option key={app.id} value={app.id}>
                            {appLabel(app)}
                          </option>
                        ))}
                      </select>
                    </label>

                    {selectedAppID && currentRepositoryLoad?.loading ? (
                      <p role="status" className="text-sm">正在加载授权仓库...</p>
                    ) : null}
                    {selectedAppID && currentRepositoryLoad?.error ? (
                      <div role="alert" className="stack-sm">
                        <p className="form-error">{currentRepositoryLoad.error}</p>
                        <div className="row">
                          <Button type="button" onClick={() => void loadRepositories(selectedAppID, true)}>
                            重试加载仓库
                          </Button>
                        </div>
                      </div>
                    ) : null}
                    {selectedAppID && !currentRepositoryLoad?.loading && !currentRepositoryLoad?.error && Object.prototype.hasOwnProperty.call(repositoriesByApp, selectedAppID) ? (
                      selectedAppRepositories.length === 0 ? (
                        <p className="text-sm">此 GitHub App 没有授权仓库</p>
                      ) : (
                        <SearchableSelect
                          label="授权仓库"
                          options={filteredRepositories}
                          value={selectedRepository}
                          query={repositoryQuery}
                          onQueryChange={(query) => {
                            setRepositoryQuery(query);
                            if (selectedRepository && query !== selectedRepository.full_name) setSelectedRepository(null);
                          }}
                          onChange={setSelectedRepository}
                          getOptionKey={(repo) => repo.full_name}
                          getOptionLabel={(repo) => repo.full_name}
                          emptyText={repositoryQuery.trim() ? "没有匹配的仓库" : "没有可添加的授权仓库"}
                          placeholder="搜索 owner/repo"
                        />
                      )
                    ) : null}

                    {selectedRepository ? (
                      <p className="repository-preview">
                        默认分支：{selectedRepository.default_branch} · 可见性：{selectedRepository.visibility}
                      </p>
                    ) : null}
                    <div className="row">
                      <Button
                        type="submit"
                        icon={<Plus size={14} strokeWidth={1.8} />}
                        disabled={!selectedRepository || pendingRepo === repositoryKey("github_app", selectedRepository?.full_name ?? "")}
                      >
                        添加仓库
                      </Button>
                    </div>
                  </>
                )}
              </form>
            ) : (
              <form className="stack-sm" onSubmit={handleAddPublicRepository}>
                <div className="field">
                  <label className="field-label" htmlFor="public-repo">公共仓库 URL 或 owner/repo</label>
                  <input
                    id="public-repo"
                    value={publicRepo}
                    onChange={(event) => setPublicRepo(event.target.value)}
                    placeholder="https://github.com/owner/repo"
                    autoComplete="off"
                  />
                  <span className="field-hint">添加仓库访问本身，不会创建 Issue 同步规则。</span>
                </div>
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
            )}
          </Card>
        </div>

        <div className="col col--fill">
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

function appLabel(app: GitHubAppView): string {
  const slug = app.app_slug || `App ${app.app_id}`;
  return app.installation_account_login ? `${slug} · ${app.installation_account_login}` : slug;
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

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}
