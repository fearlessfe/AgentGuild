import { useQuery } from "@tanstack/react-query";
import { Activity, ArrowRight, Bot, ListTodo, Plus, ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";
import { listTasks, type TaskStatus, type TaskView } from "../../api/client";
import { listAgents } from "../agents/agents.api";
import type { AgentView } from "../agents/agents.types";
import { ButtonLink, MetricGrid, StatusChip } from "../../ui";
import { PageHeader } from "../../ui/PageHeader";

const statusLabel: Record<TaskStatus, string> = {
  draft: "草稿",
  open: "待领取",
  claimed: "待审核",
  in_progress: "进行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
};

const statusTone: Record<TaskStatus, "action" | "success" | "warning" | "danger" | "info" | "neutral"> = {
  draft: "neutral",
  open: "action",
  claimed: "warning",
  in_progress: "info",
  completed: "success",
  cancelled: "danger",
  expired: "danger",
};

function taskStatusCount(tasks: TaskView[], status: TaskStatus) {
  return tasks.filter((task) => task.status === status).length;
}

function agentStatusLabel(agent: AgentView) {
  return agent.status.replace(/_/g, " ");
}

export function DashboardPage() {
  const tasksQuery = useQuery({
    queryKey: ["dashboard", "tasks"],
    queryFn: () => listTasks({ limit: 20 }),
  });
  const agentsQuery = useQuery({
    queryKey: ["dashboard", "agents"],
    queryFn: listAgents,
  });

  const tasks = tasksQuery.data?.data.items ?? [];
  const agents = agentsQuery.data?.data.items ?? [];
  const activeTasks = tasks.filter((task) => ["open", "claimed", "in_progress"].includes(task.status)).length;
  const activeAgents = agents.filter((agent) => agent.status === "active").length;
  const loading = tasksQuery.isPending || agentsQuery.isPending;

  return (
    <div className="stack dashboard-page">
      <PageHeader
        title="总览"
        sub="从这里开始：查看当前工作、管理 Agent，或进入具体模块。"
        actions={
          <div className="page-actions">
            <ButtonLink to="/tasks" variant="primary" icon={<ListTodo size={15} />}>
              查看任务
            </ButtonLink>
            <ButtonLink to="/agents/new" icon={<Plus size={15} />}>
              注册 Agent
            </ButtonLink>
          </div>
        }
      />

      <section className="dashboard-welcome" aria-labelledby="dashboard-welcome-title">
        <div>
          <span className="dashboard-kicker">AGENTGUILD WORKSPACE</span>
          <h2 id="dashboard-welcome-title">今天要推进哪一项工作？</h2>
          <p>从任务列表开始，查看可领取的工作；也可以先确认你的 Agent 是否已经在线。</p>
        </div>
        <div className="dashboard-welcome-icon" aria-hidden="true"><Activity size={26} /></div>
      </section>

      <MetricGrid
        metrics={[
          { label: "开放任务", value: loading ? "—" : taskStatusCount(tasks, "open") },
          { label: "进行中的任务", value: loading ? "—" : activeTasks, positive: activeTasks > 0 },
          { label: "已注册 Agents", value: loading ? "—" : agents.length },
          { label: "在线 Agents", value: loading ? "—" : activeAgents, positive: activeAgents > 0 },
        ]}
      />

      <div className="dashboard-grid">
        <section className="card dashboard-panel" aria-labelledby="dashboard-tasks-title">
          <div className="dashboard-panel-head">
            <div>
              <span className="dashboard-panel-kicker">WORK QUEUE</span>
              <h2 id="dashboard-tasks-title">最近任务</h2>
            </div>
            <Link className="dashboard-panel-link" to="/tasks">全部任务 <ArrowRight size={14} /></Link>
          </div>
          <div className="dashboard-list">
            {tasks.slice(0, 5).map((task) => (
              <Link className="dashboard-list-row" to={`/tasks/${encodeURIComponent(task.id)}`} key={task.id}>
                <span className="dashboard-list-main">
                  <code>{task.id}</code>
                  <strong>{task.title}</strong>
                  <small>{task.publisher_agent_version_id} · {task.type}</small>
                </span>
                <StatusChip tone={statusTone[task.status]}>{statusLabel[task.status]}</StatusChip>
              </Link>
            ))}
            {!loading && tasks.length === 0 ? <p className="dashboard-empty">暂无任务</p> : null}
          </div>
        </section>

        <section className="card dashboard-panel" aria-labelledby="dashboard-agents-title">
          <div className="dashboard-panel-head">
            <div>
              <span className="dashboard-panel-kicker">AGENT ROSTER</span>
              <h2 id="dashboard-agents-title">已注册 Agents</h2>
            </div>
            <Link className="dashboard-panel-link" to="/agents">全部 Agents <ArrowRight size={14} /></Link>
          </div>
          <div className="dashboard-list">
            {agents.slice(0, 5).map((agent) => (
              <Link className="dashboard-list-row" to={`/agents/${encodeURIComponent(agent.id)}`} key={agent.id}>
                <span className="dashboard-agent-avatar" aria-hidden="true"><Bot size={15} /></span>
                <span className="dashboard-list-main">
                  <strong>{agent.name}</strong>
                  <small>{agent.team ?? "未分配团队"} · {agent.owner_email}</small>
                </span>
                <span className={`dashboard-agent-status ${agent.status}`}><span />{agentStatusLabel(agent)}</span>
              </Link>
            ))}
            {!loading && agents.length === 0 ? <p className="dashboard-empty">还没有注册 Agent</p> : null}
          </div>
        </section>
      </div>

      <section className="dashboard-next" aria-labelledby="dashboard-next-title">
        <div className="dashboard-next-icon" aria-hidden="true"><ShieldCheck size={18} /></div>
        <div>
          <h2 id="dashboard-next-title">接下来可以做什么</h2>
          <p>配置仓库后生成任务，或注册一个 Agent 开始接入执行。</p>
        </div>
        <div className="dashboard-next-actions">
          <ButtonLink to="/repositories" icon={<ArrowRight size={14} />}>配置仓库</ButtonLink>
          <ButtonLink to="/agents/new" icon={<ArrowRight size={14} />}>注册 Agent</ButtonLink>
        </div>
      </section>
    </div>
  );
}
