import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getAgent, updateAgentStatus } from "./agents.api";
import type { AgentAction, AgentStatus, AgentView } from "./agents.types";

function formatStatus(status: AgentStatus) {
  return status.replace(/_/g, " ");
}

function formatDateTime(iso?: string) {
  if (!iso) return "—";
  const value = new Date(iso);
  if (Number.isNaN(value.getTime())) return iso;
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${value.getUTCFullYear()}-${pad(value.getUTCMonth() + 1)}-${pad(value.getUTCDate())} ${pad(value.getUTCHours())}:${pad(value.getUTCMinutes())}`;
}

function formatBudget(agent: AgentView) {
  if (!agent.budget_cents) return "未设置";
  return `${(agent.budget_cents / 100).toFixed(2)} ${agent.budget_currency ?? "USD"}`;
}

export function AgentDetail({ agentId }: { agentId: string }) {
  const queryClient = useQueryClient();
  const agentQuery = useQuery({
    queryKey: ["agent", agentId],
    queryFn: () => getAgent(agentId),
  });

  const actionMutation = useMutation({
    mutationFn: (action: AgentAction) => updateAgentStatus(agentId, action),
    onSuccess: (result) => {
      queryClient.setQueryData(["agent", agentId], result);
      queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
  });

  async function submitAction(action: AgentAction) {
    if (action === "revoke" && !window.confirm("撤销后将永久停用此 Agent。继续吗？")) {
      return;
    }
    await actionMutation.mutateAsync(action);
  }

  if (agentQuery.isPending) return <div className="loading">加载 Agent 详情…</div>;
  if (agentQuery.isError) return <div className="error">无法读取 Agent：{agentQuery.error.message}</div>;

  const agent = agentQuery.data.data;
  const canSuspend = agent.status === "active";
  const canResume = agent.status === "suspended";
  const canRevoke = agent.status !== "revoked";

  return (
    <section className="agent-detail-panel" aria-label="Agent 详情">
      <div className="detail-heading">
        <div>
          <span className="agent-detail-id">{agent.id}</span>
          <h2>{agent.name}</h2>
        </div>
        <span className={`detail-status ${agent.status}`}>{formatStatus(agent.status)}</span>
      </div>
      {agent.description ? <p className="agent-detail-description">{agent.description}</p> : null}

      <div className="agent-actions">
        {canSuspend ? (
          <button type="button" className="secondary-action" onClick={() => submitAction("suspend")} disabled={actionMutation.isPending}>
            暂停 Agent
          </button>
        ) : null}
        {canResume ? (
          <button type="button" className="secondary-action" onClick={() => submitAction("resume")} disabled={actionMutation.isPending}>
            恢复 Agent
          </button>
        ) : null}
        {canRevoke ? (
          <button type="button" className="danger-action" onClick={() => submitAction("revoke")} disabled={actionMutation.isPending}>
            撤销 Agent
          </button>
        ) : null}
      </div>

      <dl className="agent-meta-grid">
        <div>
          <dt>Status</dt>
          <dd>{formatStatus(agent.status)}</dd>
        </div>
        <div>
          <dt>Owner</dt>
          <dd>{agent.owner_email}</dd>
        </div>
        <div>
          <dt>Team</dt>
          <dd>{agent.team ?? "未分配"}</dd>
        </div>
        <div>
          <dt>Budget</dt>
          <dd>{formatBudget(agent)}</dd>
        </div>
        <div>
          <dt>Scopes</dt>
          <dd>{agent.scopes.join(", ")}</dd>
        </div>
        <div>
          <dt>Repo Scope</dt>
          <dd>{agent.repo_scope?.join(", ") || "全部仓库"}</dd>
        </div>
        <div>
          <dt>Created</dt>
          <dd>{formatDateTime(agent.created_at)}</dd>
        </div>
        <div>
          <dt>Last Seen</dt>
          <dd>{formatDateTime(agent.last_seen_at)}</dd>
        </div>
      </dl>

      {actionMutation.isError ? <p className="error">{actionMutation.error.message}</p> : null}
      {actionMutation.isPending ? <p className="loading">更新状态中…</p> : null}
    </section>
  );
}
