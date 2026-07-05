import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { type FormEvent, useMemo, useState } from "react";
import { getReputationProjection } from "./reputation.api";
import type { ProjectionView } from "./reputation.types";

function formatPercent(value?: number): string {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return "—";
  }
  return `${(value * 100).toFixed(1)}%`;
}

function formatCost(cents?: number): string {
  if (cents === undefined || cents === null || Number.isNaN(cents)) {
    return "—";
  }
  return `$${(cents / 100).toFixed(2)}`;
}

function formatLatency(ms?: number): string {
  if (ms === undefined || ms === null || Number.isNaN(ms)) {
    return "—";
  }
  return `${(ms / 1000).toFixed(2)} s`;
}

function sampleSizeLabel(hint: ProjectionView["sample_size_hint"]): string {
  switch (hint) {
    case "low":
      return "低（样本不足）";
    case "medium":
      return "中";
    case "high":
      return "高";
    default:
      return hint;
  }
}

export function ReputationPage() {
  const [searchParams, setSearchParams] = useSearchParams();

  const [form, setForm] = useState({
    agentVersionId: searchParams.get("agent_version_id") ?? "",
    capability: searchParams.get("capability") ?? "",
    taskType: searchParams.get("task_type") ?? "",
  });

  const filters = useMemo(
    () => ({
      agentVersionId: searchParams.get("agent_version_id") ?? "",
      capability: searchParams.get("capability") ?? "",
      taskType: searchParams.get("task_type") ?? "",
    }),
    [searchParams],
  );

  const query = useQuery({
    queryKey: ["reputation", filters],
    queryFn: () => getReputationProjection(filters),
    enabled: Boolean(filters.agentVersionId && filters.capability && filters.taskType),
    refetchOnWindowFocus: false,
  });

  function handleSubmit(event: FormEvent) {
    event.preventDefault();

    const next = new URLSearchParams();
    if (form.agentVersionId) next.set("agent_version_id", form.agentVersionId);
    if (form.capability) next.set("capability", form.capability);
    if (form.taskType) next.set("task_type", form.taskType);
    setSearchParams(next);
  }

  const hasFilters = Boolean(filters.agentVersionId && filters.capability && filters.taskType);

  return (
    <div className="reputation-page">
      <form className="reputation-filters" onSubmit={handleSubmit} aria-label="声望查询条件">
        <label>
          Agent Version ID
          <input
            type="text"
            value={form.agentVersionId}
            onChange={(e) => setForm((prev) => ({ ...prev, agentVersionId: e.target.value }))}
            placeholder="例如 agent-v12"
          />
        </label>
        <label>
          Capability
          <input
            type="text"
            value={form.capability}
            onChange={(e) => setForm((prev) => ({ ...prev, capability: e.target.value }))}
            placeholder="例如 code-review"
          />
        </label>
        <label>
          Task Type
          <input
            type="text"
            value={form.taskType}
            onChange={(e) => setForm((prev) => ({ ...prev, taskType: e.target.value }))}
            placeholder="例如 typescript"
          />
        </label>
        <button type="submit" className="primary-action">
          查询
        </button>
      </form>

      {!hasFilters ? (
        <div className="reputation-empty">输入 Agent Version ID、Capability 与 Task Type 后查询声望投影。</div>
      ) : query.isPending ? (
        <div className="reputation-loading">正在加载声望投影…</div>
      ) : query.isError ? (
        <div className="reputation-error" role="alert">
          加载失败：{query.error.message}
        </div>
      ) : query.data ? (
        <div className="reputation-metrics">
          <div className="metric-card">
            <span className="metric-label">总评审数</span>
            <span className="metric-value">{query.data.data.total_reviews}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">通过率</span>
            <span className="metric-value">{formatPercent(query.data.data.pass_rate)}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">返工率</span>
            <span className="metric-value">{formatPercent(query.data.data.rework_rate)}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">平均成本</span>
            <span className="metric-value">{formatCost(query.data.data.avg_review_cost_cents)}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">平均延迟</span>
            <span className="metric-value">{formatLatency(query.data.data.avg_review_latency_ms)}</span>
          </div>
          <div className="metric-card">
            <span className="metric-label">样本可信度</span>
            <span className="metric-value">{sampleSizeLabel(query.data.data.sample_size_hint)}</span>
          </div>
        </div>
      ) : null}
    </div>
  );
}
