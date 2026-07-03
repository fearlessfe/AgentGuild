import { useState, type ReactNode } from "react";
import { Navigate, NavLink, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import { AgentDetail } from "../features/agents/AgentDetail";
import { AgentList } from "../features/agents/AgentList";
import { AgentRegister } from "../features/agents/AgentRegister";
import { AgentTokenReveal } from "../features/agents/AgentTokenReveal";
import type { RegisterAgentResponse } from "../features/agents/agents.types";
import { TaskDetail } from "../features/tasks/TaskDetail";
import { TaskList } from "../features/tasks/TaskList";

export function AppShell() {
  const location = useLocation();
  const isAgentsRoute = location.pathname.startsWith("/agents");

  return (
    <div className="app-shell">
      <nav className="rail" aria-label="主导航">
        <b className="logo">AG</b>
        <span>☷</span>
        <span>⌂</span>
        <NavLink to="/tasks" className={({ isActive }) => (isActive ? "active" : undefined)} aria-label="任务">
          ▣
        </NavLink>
        <span>⌁</span>
        <span>♢</span>
        <NavLink to="/agents" className={({ isActive }) => (isActive ? "active" : undefined)} aria-label="Agents">
          ◉
        </NavLink>
        <span>⚙</span>
        <i />
        <span>?</span>
        <span>⌁</span>
      </nav>
      <div className="workspace">
        <header className="topbar">
          <strong>AgentGuild</strong>
          <button>▣　Billing Platform　⌄</button>
          <span>{isAgentsRoute ? "Agents" : "任务"}</span>
          <label>⌘ K　 搜索或执行命令…　⌕</label>
          <span className="avatar">👨🏻</span>
          <b>周昊然　⌄</b>
        </header>
        <div className="contextbar">
          <span>⌂　仓库　<b>billing-service</b></span>
          <span>GitLab MR　<b>!284 ↗</b></span>
          <span>Commit　<b>a1b2c3d</b></span>
          <span>Agent　<b>{isAgentsRoute ? "◉ identity workspace" : "▣ Atlas v12"}</b></span>
          <span>状态　<b className={isAgentsRoute ? "success" : "warning"}>● {isAgentsRoute ? "healthy" : "waiting review"}</b></span>
          <span>耗时　<b>2h37m</b></span>
          <span>成本　<b>$0.142</b></span>
        </div>
        <main>
          <Routes>
            <Route path="/tasks" element={<Workbench />} />
            <Route path="/tasks/:taskId" element={<Workbench />} />
            <Route path="/agents" element={<AgentsWorkspace />} />
            <Route path="/agents/new" element={<AgentRegistrationWorkspace />} />
            <Route path="/agents/:agentId" element={<AgentDetailWorkspace />} />
            <Route path="*" element={<Navigate to="/agents" replace />} />
          </Routes>
        </main>
        <footer>
          <b>CI 与运行记录　⌄</b>
          <span>✓　CI Workflow　　#918374　 <em>成功</em>　 14m 21s　 刚刚</span>
          <span className="warning">▲　隐藏用例　18 / 20 未通过</span>
          <a>查看详情 ↗</a>
        </footer>
      </div>
    </div>
  );
}

function PageHeader({
  title,
  subtitle,
  action,
}: {
  title: string;
  subtitle: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-title">
      <div>
        <h1>{title}</h1>
        <span>{subtitle}</span>
      </div>
      {action ?? <span className="readonly">只读观察模式</span>}
    </div>
  );
}

function Workbench() {
  const { taskId } = useParams();
  const navigate = useNavigate();
  return (
    <div className="observer">
      <PageHeader title="任务" subtitle="发布、分配并追踪 Agent 的代码任务" />
      <div className="split">
        <div className="list-pane">
          <TaskList />
        </div>
        {taskId ? (
          <TaskDetail key={taskId} taskId={taskId} onClose={() => navigate("/tasks")} />
        ) : (
          <aside className="task-detail empty">
            <p>选择左侧任务查看详情</p>
          </aside>
        )}
      </div>
    </div>
  );
}

function AgentsWorkspace() {
  return (
    <div className="observer">
      <PageHeader title="Agents" subtitle="注册、启用并管理执行 Agent 的身份与状态" action={<NavLink className="primary-action" to="/agents/new">新增 Agent</NavLink>} />
      <div className="agent-page-body">
        <div className="list-pane">
          <AgentList />
        </div>
      </div>
    </div>
  );
}

function AgentRegistrationWorkspace() {
  const [tokenView, setTokenView] = useState<RegisterAgentResponse | null>(null);

  return (
    <div className="observer">
      <PageHeader title="注册 Agent" subtitle="创建新 Agent，并在注册完成后一次性领取 activation token" action={<NavLink className="secondary-action" to="/agents">返回列表</NavLink>} />
      <div className="agent-page-body">
        <div className="agent-register-layout">
          {tokenView ? <AgentTokenReveal tokenView={tokenView} onDismiss={() => setTokenView(null)} /> : null}
          <AgentRegister onRegistered={setTokenView} />
        </div>
      </div>
    </div>
  );
}

function AgentDetailWorkspace() {
  const { agentId } = useParams();

  if (!agentId) {
    return <Navigate to="/agents" replace />;
  }

  return (
    <div className="observer">
      <PageHeader title="Agent 详情" subtitle="查看单个 Agent 的状态、权限与生命周期操作" action={<NavLink className="secondary-action" to="/agents">返回列表</NavLink>} />
      <div className="agent-page-body">
        <AgentDetail agentId={agentId} />
      </div>
    </div>
  );
}
