import { useQuery } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";
import type { AgentStatus, AgentView } from "./agents.types";
import { listAgents } from "./agents.api";

const statusOptions: Array<{ value?: AgentStatus; label: string }> = [
  { label: "全部状态" },
  { value: "pending_activation", label: "pending activation" },
  { value: "active", label: "active" },
  { value: "suspended", label: "suspended" },
  { value: "revoked", label: "revoked" },
];

function formatStatus(status: AgentStatus) {
  return status.replace(/_/g, " ");
}

function formatDateTime(iso?: string) {
  if (!iso) return "从未上线";
  const value = new Date(iso);
  if (Number.isNaN(value.getTime())) return iso;
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${value.getUTCFullYear()}-${pad(value.getUTCMonth() + 1)}-${pad(value.getUTCDate())} ${pad(value.getUTCHours())}:${pad(value.getUTCMinutes())}`;
}

function scopeSummary(agent: AgentView) {
  const scopes = agent.scopes.join(", ");
  if (agent.repo_scope && agent.repo_scope.length > 0) {
    return `${scopes} · ${agent.repo_scope.join(", ")}`;
  }
  return scopes;
}

export function AgentList() {
  const [searchParams, setSearchParams] = useSearchParams();
  const status = (searchParams.get("status") as AgentStatus | null) ?? undefined;

  const query = useQuery({
    queryKey: ["agents"],
    queryFn: () => listAgents(),
  });

  function updateStatus(nextStatus?: AgentStatus) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (nextStatus) next.set("status", nextStatus);
      else next.delete("status");
      return next;
    });
  }

  if (query.isPending) return <div className="loading">正在同步 Agents…</div>;
  if (query.isError) return <div className="error">无法读取 Agents：{query.error.message}</div>;

  const agents = status ? query.data.data.items.filter((agent) => agent.status === status) : query.data.data.items;

  return (
    <>
      <div className="agent-toolbar">
        <label htmlFor="agent-status-filter">
          状态
          <select
            id="agent-status-filter"
            value={status ?? ""}
            onChange={(event) => updateStatus((event.target.value as AgentStatus) || undefined)}
          >
            {statusOptions.map((option) => (
              <option key={option.value ?? "all"} value={option.value ?? ""}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <Link className="primary-action" to="/agents/new">
          注册 Agent
        </Link>
      </div>

      <div className="agent-table" aria-label="Agent 列表">
        <div className="agent-row agent-header">
          <span>状态</span>
          <span>名称</span>
          <span>Owner</span>
          <span>Team</span>
          <span>最后在线</span>
          <span>Scopes</span>
          <span />
        </div>
        {agents.map((agent) => (
          <Link className="agent-row" key={agent.id} to={`/console/agents/${agent.id}`} aria-label={`查看 ${agent.name}`}>
            <span className="agent-status-cell">
              <span className={`status-dot ${agent.status}`} aria-hidden="true" />
              <span className={`agent-status-text ${agent.status}`}>{formatStatus(agent.status)}</span>
            </span>
            <span className="agent-name" title={agent.name}>
              {agent.name}
            </span>
            <span className="agent-owner" title={agent.owner_email}>
              {agent.owner_email}
            </span>
            <span className="agent-team" title={agent.team ?? "未分配"}>
              {agent.team ?? "未分配"}
            </span>
            <span className="agent-last-seen">{formatDateTime(agent.last_seen_at)}</span>
            <span className="agent-scopes" title={scopeSummary(agent)}>
              {scopeSummary(agent)}
            </span>
            <span className="agent-go" aria-hidden="true">
              <ChevronRight size={14} strokeWidth={2} />
            </span>
          </Link>
        ))}
      </div>
      <div className="agent-mobile-list" aria-label="移动 Agent 列表">
        {agents.map((agent) => (
          <Link className="agent-mobile-card" key={agent.id} to={`/console/agents/${agent.id}`} aria-label={`查看 ${agent.name}`}>
            <div className="row-between">
              <strong>{agent.name}</strong>
              <span className="agent-status-cell">
                <span className={`status-dot ${agent.status}`} aria-hidden="true" />
                <span className={`agent-status-text ${agent.status}`}>{formatStatus(agent.status)}</span>
              </span>
            </div>
            <span className="muted text-sm">{agent.owner_email}</span>
            <span className="faint text-xs">{agent.team ?? "未分配"}</span>
          </Link>
        ))}
      </div>
    </>
  );
}
