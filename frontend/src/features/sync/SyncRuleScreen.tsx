import { useState, useEffect } from "react";
import type { FormEvent } from "react";
import { PageHeader, Card, Button, StatusChip, DenseTable } from "../../ui";
import type { DenseRow } from "../../ui";
import type { Repository, SyncRule, SyncResult } from "../../api/client";
import { listRepositories, listSyncRules, createSyncRule, updateSyncRule, deleteSyncRule, runSyncRule } from "../../api/client";
import { SyncResultScreen } from "./SyncResultScreen";

export function SyncRuleScreen() {
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [syncRules, setSyncRules] = useState<SyncRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [syncResult, setSyncResult] = useState<SyncResult | null>(null);
  const [runningRuleId, setRunningRuleId] = useState<string | null>(null);
  const [publicRepo, setPublicRepo] = useState("");
  const [publicLabels, setPublicLabels] = useState("");
  const [creatingPublicRule, setCreatingPublicRule] = useState(false);

  useEffect(() => {
    async function loadData() {
      setLoading(true);
      const [reposResult, rulesResult] = await Promise.allSettled([listRepositories(), listSyncRules()]);
      const errors: string[] = [];

      if (reposResult.status === "fulfilled") {
        setRepositories(reposResult.value.data.items);
      } else {
        setRepositories([]);
        errors.push(`仓库加载失败：${reposResult.reason instanceof Error ? reposResult.reason.message : "未知错误"}`);
      }

      if (rulesResult.status === "fulfilled") {
        setSyncRules(rulesResult.value.data.items);
      } else {
        setSyncRules([]);
        errors.push(`规则加载失败：${rulesResult.reason instanceof Error ? rulesResult.reason.message : "未知错误"}`);
      }

      setError(errors.length > 0 ? errors.join("；") : null);
      setLoading(false);
    }
    loadData();
  }, []);

  const handleToggleEnabled = async (rule: SyncRule) => {
    try {
      const updated = await updateSyncRule(rule.id, {
        repo: rule.repo,
        include_labels: labels(rule.include_labels),
        exclude_labels: labels(rule.exclude_labels),
        issue_state: rule.issue_state,
        task_type: rule.task_type,
        default_priority: rule.default_priority,
        dedupe_strategy: rule.dedupe_strategy,
        source_auth: rule.source_auth,
        enabled: !rule.enabled,
      });
      setSyncRules(syncRules.map((r) => (r.id === rule.id ? updated.data : r)));
    } catch (err) {
      alert(`切换状态失败: ${err instanceof Error ? err.message : "未知错误"}`);
    }
  };

  const handleDelete = async (ruleId: string) => {
    if (!confirm("确定要删除此同步规则吗？")) return;
    try {
      await deleteSyncRule(ruleId);
      setSyncRules(syncRules.filter((r) => r.id !== ruleId));
    } catch (err) {
      alert(`删除失败: ${err instanceof Error ? err.message : "未知错误"}`);
    }
  };

  const handleRunSync = async (ruleId: string) => {
    try {
      setRunningRuleId(ruleId);
      const result = await runSyncRule(ruleId);
      setSyncResult(result.data);
    } catch (err) {
      alert(`运行同步失败: ${err instanceof Error ? err.message : "未知错误"}`);
    } finally {
      setRunningRuleId(null);
    }
  };

  const handleCreatePublicRule = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const repo = publicRepo.trim();
    if (!repo) return;
    try {
      setCreatingPublicRule(true);
      const created = await createSyncRule({
        repo,
        include_labels: parseLabels(publicLabels),
        exclude_labels: [],
        issue_state: "open",
        task_type: "github_issue",
        default_priority: "normal",
        dedupe_strategy: "update",
        source_auth: "public",
      });
      setSyncRules([created.data, ...syncRules]);
      setPublicRepo("");
      setPublicLabels("");
    } catch (err) {
      alert(`创建公共同步规则失败: ${err instanceof Error ? err.message : "未知错误"}`);
    } finally {
      setCreatingPublicRule(false);
    }
  };

  if (syncResult) {
    return <SyncResultScreen result={syncResult} onClose={() => setSyncResult(null)} />;
  }

  const repoRows: DenseRow[] = repositories.map((repo) => {
    const hasRule = syncRules.some((r) => r.repo === repo.full_name && r.enabled);
    return {
      cells: [
        repo.full_name,
        <StatusChip tone={hasRule ? "success" : "neutral"}>{hasRule ? "已启用" : "未启用"}</StatusChip>,
        <code>{repo.default_branch}</code>,
        repo.visibility,
      ],
    };
  });

  const ruleRows: DenseRow[] = syncRules.map((rule) => ({
    cells: [
      rule.repo,
      <StatusChip tone={rule.source_auth === "public" ? "info" : "neutral"}>
        {rule.source_auth === "public" ? "Public" : "App"}
      </StatusChip>,
      <StatusChip tone={rule.enabled ? "success" : "neutral"}>{rule.enabled ? "已启用" : "已停用"}</StatusChip>,
      labels(rule.include_labels).join(", ") || "—",
      labels(rule.exclude_labels).join(", ") || "—",
      rule.last_synced_at ? new Date(rule.last_synced_at).toLocaleString("zh-CN") : "从未运行",
      <span className="row">
        <Button variant="ghost" onClick={() => handleToggleEnabled(rule)}>
          {rule.enabled ? "停用" : "启用"}
        </Button>
        <Button variant="ghost" onClick={() => handleRunSync(rule.id)} disabled={runningRuleId === rule.id}>
          {runningRuleId === rule.id ? "运行中..." : "立即同步"}
        </Button>
        <Button variant="ghost" onClick={() => handleDelete(rule.id)}>
          删除
        </Button>
      </span>,
    ],
  }));

  return (
    <div className="stack">
      <PageHeader
        title="仓库与同步规则"
        sub="选择要纳入治理的仓库，并用规则驱动 Issue → Task 同步。"
      />
      {error && (
        <Card title="错误" sub={error}>
          <p className="text-sm">{error}</p>
        </Card>
      )}
      {loading ? (
        <Card title="加载中...">
          <p className="text-sm">正在加载数据...</p>
        </Card>
      ) : (
        <div className="split-2">
          <div className="col col--fill">
            <Card title="安装仓库" sub={`${repositories.length} 个仓库`} pad={false}>
              <DenseTable
                columns={["仓库", "同步", "默认分支", "可见性"]}
                rows={repoRows}
                caption="安装仓库列表"
              />
            </Card>
          </div>
          <div className="col">
            <Card title="公共仓库同步" sub="无需安装 GitHub App">
              <form className="stack" onSubmit={handleCreatePublicRule}>
                <label className="field">
                  <span className="field-label">仓库</span>
                  <input
                    value={publicRepo}
                    onChange={(event) => setPublicRepo(event.target.value)}
                    placeholder="owner/repo"
                    autoComplete="off"
                  />
                </label>
                <label className="field">
                  <span className="field-label">包含标签</span>
                  <input
                    value={publicLabels}
                    onChange={(event) => setPublicLabels(event.target.value)}
                    placeholder="good first issue, bug"
                    autoComplete="off"
                  />
                  <span className="field-hint">可选，逗号分隔；留空同步 open issue。</span>
                </label>
                <div className="row">
                  <Button type="submit" disabled={creatingPublicRule || publicRepo.trim() === ""}>
                    {creatingPublicRule ? "创建中..." : "添加公共规则"}
                  </Button>
                </div>
              </form>
            </Card>
            <Card title="同步规则" sub={`${syncRules.length} 条规则`} pad={false}>
              <DenseTable
                columns={["仓库", "来源", "状态", "包含标签", "排除标签", "最后同步", "操作"]}
                rows={ruleRows}
                caption="同步规则列表"
              />
            </Card>
          </div>
        </div>
      )}
    </div>
  );
}

function labels(value: string[] | null | undefined): string[] {
  return Array.isArray(value) ? value : [];
}

function parseLabels(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}
