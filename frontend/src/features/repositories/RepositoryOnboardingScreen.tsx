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
  const [appsLoad, setAppsLoad] = useState<RepositoryLoadState>({ loading: true, error: null });
  const [summaryLoad, setSummaryLoad] = useState<RepositoryLoadState>({ loading: true, error: null });
  const [error, setError] = useState<string | null>(null);
  const [pendingRepo, setPendingRepo] = useState<string | null>(null);
  const requestVersions = useRef<Record<string, number>>({});
  const initialRequestVersions = useRef({ apps: 0, summary: 0 });
  const mutationLock = useRef(false);
  const mutationKeys = useRef<Record<string, string>>({});
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    void loadApps();
    void loadSummary();
    return () => {
      mounted.current = false;
    };
  }, []);

  const loadApps = async () => {
    const version = initialRequestVersions.current.apps + 1;
    initialRequestVersions.current.apps = version;
    setAppsLoad({ loading: true, error: null });
    try {
      const response = await client.listGitHubApps();
      if (!mounted.current || initialRequestVersions.current.apps !== version) return;
      setApps(response.data.items);
      setAppsLoad({ loading: false, error: null });
    } catch (err) {
      if (mounted.current && initialRequestVersions.current.apps === version) {
        setAppsLoad({ loading: false, error: errorMessage(err, "加载 GitHub Apps 失败") });
      }
    }
  };

  const loadSummary = async () => {
    const version = initialRequestVersions.current.summary + 1;
    initialRequestVersions.current.summary = version;
    setSummaryLoad({ loading: true, error: null });
    try {
      const response = await client.getRepositoryOnboarding();
      if (!mounted.current || initialRequestVersions.current.summary !== version) return;
      setSummary(response.data);
      setSummaryLoad({ loading: false, error: null });
    } catch (err) {
      if (mounted.current && initialRequestVersions.current.summary === version) {
        setSummaryLoad({ loading: false, error: errorMessage(err, "加载仓库清单失败") });
      }
    }
  };

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
  const loading = appsLoad.loading || summaryLoad.loading;
  const inventoryReady = summary !== null && !summaryLoad.loading && !summaryLoad.error;
  const isMutating = pendingRepo !== null;

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
    if (mutationLock.current) return;
    setSourceMode(nextSource);
    setSelectedAppID("");
    setPublicRepo("");
    clearRepositoryChoice();
  };

  const handleAppChange = (appID: string) => {
    if (mutationLock.current) return;
    setSelectedAppID(appID);
    clearRepositoryChoice();
    if (appID) void loadRepositories(appID);
  };

  const handleAddAppRepository = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!inventoryReady || !selectedAppID || !selectedRepository) return;
    const logicalKey = repositoryKey("github_app", `${selectedAppID}:${selectedRepository.full_name}`);
    if (!beginMutation(logicalKey)) return;
    try {
      const response = await client.addGitHubAppRepository(selectedAppID, selectedRepository.full_name, mutationOptions(logicalKey));
      settleMutation(logicalKey);
      setSummary((current) => appendOnboardedRepository(current, response.data));
      clearRepositoryChoice();
    } catch (err) {
      if (!client.shouldRetainMutationKey(err)) settleMutation(logicalKey);
      setError(errorMessage(err, "添加 GitHub App 仓库失败"));
      if (err instanceof client.ApiError && err.status === 409) await loadSummary();
    } finally {
      endMutation();
    }
  };

  const handleAddPublicRepository = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const repo = publicRepo.trim();
    if (!inventoryReady || !repo) return;
    const logicalKey = repositoryKey("public_github", repo);
    if (!beginMutation(logicalKey)) return;
    try {
      const response = await client.addPublicRepository(repo, mutationOptions(logicalKey));
      settleMutation(logicalKey);
      setSummary((current) => appendOnboardedRepository(current, response.data));
      setPublicRepo("");
    } catch (err) {
      if (!client.shouldRetainMutationKey(err)) settleMutation(logicalKey);
      setError(errorMessage(err, "添加公开仓库失败"));
      if (err instanceof client.ApiError && err.status === 409) await loadSummary();
    } finally {
      endMutation();
    }
  };

  const handleRemove = async (repo: RepositoryInventoryItem) => {
    if (!repo.id) return;
    const logicalKey = `delete:${repo.id}`;
    if (!beginMutation(logicalKey)) return;
    try {
      await client.removeRepository(repo.id, mutationOptions(logicalKey));
      settleMutation(logicalKey);
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
      if (!client.shouldRetainMutationKey(err)) settleMutation(logicalKey);
      setError(errorMessage(err, "移除仓库失败"));
    } finally {
      endMutation();
    }
  };

  const beginMutation = (key: string): boolean => {
    if (mutationLock.current) return false;
    mutationLock.current = true;
    setError(null);
    setPendingRepo(key);
    return true;
  };

  const mutationOptions = (logicalKey: string): client.MutationOptions => {
    mutationKeys.current[logicalKey] ??= client.createIdempotencyKey();
    return { idempotencyKey: mutationKeys.current[logicalKey] };
  };

  const settleMutation = (logicalKey: string) => {
    delete mutationKeys.current[logicalKey];
  };

  const endMutation = () => {
    mutationLock.current = false;
    setPendingRepo(null);
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
        disabled={!repo.id || isMutating}
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
      {appsLoad.error ? (
        <div role="alert" className="stack-sm">
          <p className="form-error">{appsLoad.error}</p>
          <div className="row"><Button type="button" onClick={() => void loadApps()}>重试加载 GitHub Apps</Button></div>
        </div>
      ) : null}
      {summaryLoad.error ? (
        <div role="alert" className="stack-sm">
          <p className="form-error">{summaryLoad.error}</p>
          <div className="row"><Button type="button" onClick={() => void loadSummary()}>重试加载仓库清单</Button></div>
        </div>
      ) : null}
      {error ? <p role="alert" className="form-error">{error}</p> : null}

      <div className="split-2">
        <div className="col">
          <Card className="repository-add-card" title="添加仓库" sub="选择一种仓库来源并完成接入。">
            <fieldset className="source-selector" disabled={appsLoad.loading || isMutating}>
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
                {!appsLoad.loading && !appsLoad.error && installedApps.length === 0 ? (
                  <div className="stack-sm">
                    <p className="text-sm">暂无已安装的 GitHub App</p>
                    <p className="field-hint">请先完成 GitHub App 安装，再选择其授权仓库。</p>
                    <div className="row">
                      <Button icon={<ExternalLink size={14} strokeWidth={1.8} />} type="button" onClick={() => (window.location.href = "/git-integration")}>
                        打开 Git 接入
                      </Button>
                    </div>
                  </div>
                ) : appsLoad.error ? null : (
                  <>
                    <label className="field" htmlFor="github-app-select">
                      <span className="field-label">GitHub App</span>
                      <select
                        id="github-app-select"
                        value={selectedAppID}
                        onChange={(event) => handleAppChange(event.target.value)}
                        disabled={appsLoad.loading || isMutating}
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
                          disabled={isMutating}
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
                        disabled={!inventoryReady || !selectedRepository || isMutating}
                      >
                        添加仓库
                      </Button>
                    </div>
                    {!inventoryReady ? <p className="field-hint">仓库清单确认后才能添加仓库，请重试加载仓库清单。</p> : null}
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
                    disabled={isMutating}
                  />
                  <span className="field-hint">添加仓库访问本身，不会创建 Issue 同步规则。</span>
                </div>
                <div className="row">
                  <Button
                    type="submit"
                    icon={<Plus size={14} strokeWidth={1.8} />}
                    disabled={!inventoryReady || !publicRepo.trim() || isMutating}
                  >
                    添加公开仓库
                  </Button>
                </div>
                {!inventoryReady ? <p className="field-hint">仓库清单确认后才能添加仓库，请重试加载仓库清单。</p> : null}
              </form>
            )}
          </Card>
        </div>

        <div className="col col--fill">
          <Card title="已接入仓库" sub={summary ? `${onboarded.length} 个仓库` : "仓库清单未确认"} pad={false}>
            {summary ? (
              <DenseTable
                columns={["仓库", "来源", "默认分支", "可见性", "操作"]}
                rows={onboardedRows}
                caption="已接入仓库列表"
              />
            ) : <div className="card-pad"><p className="text-sm">仓库清单尚未加载</p></div>}
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
