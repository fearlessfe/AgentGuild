import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Play, Plus, Trash2 } from "lucide-react";
import { PageHeader, Card, Button, ButtonLink, StatusChip, DenseTable } from "../../ui";
import type { DenseRow } from "../../ui";
import type { RepositoryInventoryItem, SyncRule, SyncRuleInput, SyncResult } from "../../api/client";
import {
  createIdempotencyKey,
  createSyncRule,
  deleteSyncRule,
  getRepositoryOnboarding,
  listSyncRules,
  runSyncRule,
  shouldRetainMutationKey,
  updateSyncRule,
} from "../../api/client";
import { SyncResultScreen } from "./SyncResultScreen";

type RuleForm = {
  repo: string;
  includeLabels: string;
  excludeLabels: string;
  issueState: string;
  taskType: string;
  defaultPriority: string;
  dedupeStrategy: string;
  enabled: boolean;
  runAfterCreate: boolean;
};

const INITIAL_FORM: RuleForm = {
  repo: "",
  includeLabels: "agent-task",
  excludeLabels: "blocked, wontfix",
  issueState: "open",
  taskType: "coding",
  defaultPriority: "normal",
  dedupeStrategy: "update",
  enabled: true,
  runAfterCreate: true,
};

export function SyncRuleScreen() {
  const [repositories, setRepositories] = useState<RepositoryInventoryItem[]>([]);
  const [syncRules, setSyncRules] = useState<SyncRule[]>([]);
  const [form, setForm] = useState<RuleForm>(INITIAL_FORM);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null);
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const mutationKeys = useRef<Record<string, string>>({});

  useEffect(() => {
    async function loadData() {
      setLoading(true);
      const [reposResult, rulesResult] = await Promise.allSettled([getRepositoryOnboarding(), listSyncRules()]);
      const errors: string[] = [];
      if (reposResult.status === "fulfilled") {
        const items = reposResult.value.data.onboarded_repositories.items.filter(isOnboardedRepository);
        setRepositories(items);
        setForm((current) => ({ ...current, repo: current.repo || items[0]?.full_name || "" }));
      } else {
        errors.push(`仓库加载失败：${errorMessage(reposResult.reason)}`);
      }
      if (rulesResult.status === "fulfilled") setSyncRules(rulesResult.value.data.items);
      else errors.push(`规则加载失败：${errorMessage(rulesResult.reason)}`);
      setError(errors.length > 0 ? errors.join("；") : null);
      setLoading(false);
    }
    void loadData();
  }, []);

  const selectedRepository = useMemo(
    () => repositories.find((repository) => repository.full_name === form.repo),
    [form.repo, repositories],
  );
  const sourceAuth = selectedRepository?.source_type === "public_github" ? "public" : "app";

  const handleCreate = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!selectedRepository || pendingAction) return;
    const input: SyncRuleInput = {
      repo: form.repo,
      include_labels: parseLabels(form.includeLabels),
      exclude_labels: parseLabels(form.excludeLabels),
      issue_state: form.issueState,
      task_type: form.taskType.trim(),
      default_priority: form.defaultPriority,
      dedupe_strategy: form.dedupeStrategy,
      source_auth: sourceAuth,
      enabled: form.enabled,
    };
    const signature = `create:${JSON.stringify(input)}`;
    setPendingAction("create");
    setActionError(null);
    try {
      const created = await createSyncRule(input, mutationOptions(mutationKeys.current, signature));
      settle(mutationKeys.current, signature);
      setSyncRules((current) => [created.data, ...current.filter((rule) => rule.id !== created.data.id)]);
      setForm((current) => ({ ...INITIAL_FORM, repo: current.repo }));
      if (form.runAfterCreate && created.data.enabled) {
        const runKey = `run:${created.data.id}`;
        try {
          const result = await runSyncRule(created.data.id, mutationOptions(mutationKeys.current, runKey));
          settle(mutationKeys.current, runKey);
          setSyncResult(result.data);
        } catch (err) {
          if (!shouldRetainMutationKey(err)) settle(mutationKeys.current, runKey);
          setActionError(`规则已创建，但首次同步失败：${errorMessage(err)}`);
        }
      }
    } catch (err) {
      if (!shouldRetainMutationKey(err)) settle(mutationKeys.current, signature);
      setActionError(`创建规则失败：${errorMessage(err)}`);
    } finally {
      setPendingAction(null);
    }
  };

  const handleToggleEnabled = async (rule: SyncRule) => {
    const action = `toggle:${rule.id}:${!rule.enabled}`;
    setPendingAction(action);
    setActionError(null);
    const input = ruleInput(rule, !rule.enabled);
    try {
      const updated = await updateSyncRule(rule.id, input, mutationOptions(mutationKeys.current, action));
      settle(mutationKeys.current, action);
      setSyncRules((current) => current.map((item) => (item.id === rule.id ? updated.data : item)));
    } catch (err) {
      if (!shouldRetainMutationKey(err)) settle(mutationKeys.current, action);
      setActionError(`切换状态失败：${errorMessage(err)}`);
    } finally {
      setPendingAction(null);
    }
  };

  const handleDelete = async (ruleId: string) => {
    if (!confirm("确定要删除此同步规则吗？")) return;
    const action = `delete:${ruleId}`;
    setPendingAction(action);
    setActionError(null);
    try {
      await deleteSyncRule(ruleId, mutationOptions(mutationKeys.current, action));
      settle(mutationKeys.current, action);
      setSyncRules((current) => current.filter((rule) => rule.id !== ruleId));
    } catch (err) {
      if (!shouldRetainMutationKey(err)) settle(mutationKeys.current, action);
      setActionError(`删除失败：${errorMessage(err)}`);
    } finally {
      setPendingAction(null);
    }
  };

  const handleRunSync = async (ruleId: string) => {
    const action = `run:${ruleId}`;
    setPendingAction(action);
    setActionError(null);
    try {
      const result = await runSyncRule(ruleId, mutationOptions(mutationKeys.current, action));
      settle(mutationKeys.current, action);
      setSyncResult(result.data);
    } catch (err) {
      if (!shouldRetainMutationKey(err)) settle(mutationKeys.current, action);
      setActionError(`运行同步失败：${errorMessage(err)}`);
    } finally {
      setPendingAction(null);
    }
  };

  if (syncResult) return <SyncResultScreen result={syncResult} onClose={() => setSyncResult(null)} />;

  const repoRows: DenseRow[] = repositories.map((repo) => {
    const hasRule = syncRules.some((rule) => rule.repo === repo.full_name && rule.enabled);
    return {
      key: repo.id ?? repo.full_name,
      cells: [
        repo.full_name,
        <StatusChip tone={repo.source_type === "public_github" ? "info" : "success"}>{sourceLabel(repo)}</StatusChip>,
        <StatusChip tone={hasRule ? "success" : "neutral"}>{hasRule ? "已启用" : "未启用"}</StatusChip>,
        <code>{repo.default_branch}</code>,
      ],
    };
  });

  const ruleRows: DenseRow[] = syncRules.map((rule) => ({
    key: rule.id,
    cells: [
      rule.repo,
      <StatusChip tone={rule.source_auth === "public" ? "info" : "neutral"}>{rule.source_auth === "public" ? "Public" : "App"}</StatusChip>,
      <StatusChip tone={rule.enabled ? "success" : "neutral"}>{rule.enabled ? "已启用" : "已停用"}</StatusChip>,
      labels(rule.include_labels).join(", ") || "—",
      labels(rule.exclude_labels).join(", ") || "—",
      rule.last_synced_at ? new Date(rule.last_synced_at).toLocaleString("zh-CN") : "从未运行",
      <span className="row">
        <Button variant="ghost" onClick={() => void handleToggleEnabled(rule)} disabled={pendingAction !== null}>
          {rule.enabled ? "停用" : "启用"}
        </Button>
        <Button icon={<Play size={14} />} variant="ghost" onClick={() => void handleRunSync(rule.id)} disabled={pendingAction !== null}>
          {pendingAction === `run:${rule.id}` ? "运行中..." : "立即同步"}
        </Button>
        <Button icon={<Trash2 size={14} />} variant="ghost" onClick={() => void handleDelete(rule.id)} disabled={pendingAction !== null}>
          删除
        </Button>
      </span>,
    ],
  }));

  return (
    <div className="stack">
      <PageHeader
        title="同步规则"
        sub="为已接入仓库创建规则，并运行首次 Issue → Task 同步。"
        actions={<ButtonLink to="/repositories">仓库接入</ButtonLink>}
      />
      {error ? <p role="alert" className="form-error">{error}</p> : null}
      {actionError ? <p role="alert" className="form-error">{actionError}</p> : null}
      {loading ? <p role="status" className="text-sm">正在加载同步配置...</p> : (
        <>
          <div className="split-2">
            <div className="col">
              <Card title="新建同步规则" sub="来源权限根据仓库接入方式自动匹配。">
                {repositories.length === 0 ? (
                  <div className="stack-sm">
                    <p className="text-sm">暂无已接入仓库</p>
                    <div className="row"><ButtonLink to="/repositories" variant="primary">接入仓库</ButtonLink></div>
                  </div>
                ) : (
                  <form className="rule-grid" onSubmit={handleCreate}>
                    <label className="field field-span-2" htmlFor="sync-repo">
                      <span className="field-label">仓库</span>
                      <select id="sync-repo" value={form.repo} onChange={(event) => setForm({ ...form, repo: event.target.value })} disabled={pendingAction !== null}>
                        {repositories.map((repo) => <option key={repo.id ?? repo.full_name} value={repo.full_name}>{repo.full_name}</option>)}
                      </select>
                    </label>
                    <label className="field" htmlFor="sync-source">
                      <span className="field-label">仓库来源</span>
                      <input id="sync-source" value={sourceLabel(selectedRepository)} readOnly />
                    </label>
                    <label className="field" htmlFor="sync-auth">
                      <span className="field-label">Issue 访问</span>
                      <input id="sync-auth" value={sourceAuth} readOnly />
                    </label>
                    <label className="field" htmlFor="sync-include"><span className="field-label">包含标签</span><input id="sync-include" value={form.includeLabels} onChange={(event) => setForm({ ...form, includeLabels: event.target.value })} placeholder="agent-task, bug" /></label>
                    <label className="field" htmlFor="sync-exclude"><span className="field-label">排除标签</span><input id="sync-exclude" value={form.excludeLabels} onChange={(event) => setForm({ ...form, excludeLabels: event.target.value })} placeholder="blocked, wontfix" /></label>
                    <label className="field" htmlFor="sync-state"><span className="field-label">Issue 状态</span><select id="sync-state" value={form.issueState} onChange={(event) => setForm({ ...form, issueState: event.target.value })}><option value="open">仅 Open</option><option value="closed">仅 Closed</option><option value="all">全部</option></select></label>
                    <label className="field" htmlFor="sync-type"><span className="field-label">任务类型</span><input id="sync-type" required value={form.taskType} onChange={(event) => setForm({ ...form, taskType: event.target.value })} /></label>
                    <label className="field" htmlFor="sync-priority"><span className="field-label">默认优先级</span><select id="sync-priority" value={form.defaultPriority} onChange={(event) => setForm({ ...form, defaultPriority: event.target.value })}><option value="low">Low</option><option value="normal">Normal</option><option value="high">High</option><option value="urgent">Urgent</option></select></label>
                    <label className="field" htmlFor="sync-dedupe"><span className="field-label">重复 Issue</span><select id="sync-dedupe" value={form.dedupeStrategy} onChange={(event) => setForm({ ...form, dedupeStrategy: event.target.value })}><option value="update">更新已有任务</option><option value="skip">跳过</option></select></label>
                    <label className="source-selector__option"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} /><span>创建后启用</span></label>
                    <label className="source-selector__option"><input type="checkbox" checked={form.runAfterCreate} onChange={(event) => setForm({ ...form, runAfterCreate: event.target.checked })} /><span>创建后立即同步</span></label>
                    <div className="row field-span-2"><Button icon={<Plus size={14} />} type="submit" variant="primary" disabled={!selectedRepository || pendingAction !== null}>{pendingAction === "create" ? "创建中..." : "创建规则"}</Button></div>
                  </form>
                )}
              </Card>
            </div>
            <div className="col col--fill">
              <Card title="已接入仓库" sub={`${repositories.length} 个仓库`} pad={false}>
                <DenseTable columns={["仓库", "来源", "同步", "默认分支"]} rows={repoRows} caption="已接入仓库同步状态" />
              </Card>
            </div>
          </div>
          <Card title="同步规则" sub={`${syncRules.length} 条规则`} pad={false}>
            <DenseTable columns={["仓库", "来源", "状态", "包含标签", "排除标签", "最后同步", "操作"]} rows={ruleRows} caption="同步规则列表" />
          </Card>
        </>
      )}
    </div>
  );
}

function labels(value: string[] | null | undefined): string[] { return Array.isArray(value) ? value : []; }
function parseLabels(value: string): string[] { return value.split(",").map((item) => item.trim()).filter(Boolean); }
function isOnboardedRepository(repository: RepositoryInventoryItem): boolean {
  return Boolean(repository.id && (repository.source_type === "github_app" || repository.source_type === "public_github"));
}
function sourceLabel(repository?: RepositoryInventoryItem): string {
  if (!repository) return "";
  return repository.source_type === "public_github" ? "公开 GitHub" : "GitHub App";
}
function errorMessage(error: unknown): string { return error instanceof Error ? error.message : "未知错误"; }
function mutationOptions(keys: Record<string, string>, key: string) { keys[key] ??= createIdempotencyKey(); return { idempotencyKey: keys[key] }; }
function settle(keys: Record<string, string>, key: string) { delete keys[key]; }
function ruleInput(rule: SyncRule, enabled: boolean): SyncRuleInput {
  return {
    repo: rule.repo,
    include_labels: labels(rule.include_labels),
    exclude_labels: labels(rule.exclude_labels),
    issue_state: rule.issue_state,
    task_type: rule.task_type,
    default_priority: rule.default_priority,
    dedupe_strategy: rule.dedupe_strategy,
    source_auth: rule.source_auth,
    enabled,
  };
}
