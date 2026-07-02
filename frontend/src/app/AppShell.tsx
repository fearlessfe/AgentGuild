import { Navigate, Route, Routes, useNavigate, useParams } from "react-router-dom";
import { TaskDetail } from "../features/tasks/TaskDetail";
import { TaskList } from "../features/tasks/TaskList";

export function AppShell() {
  return (
    <div className="app-shell">
      <nav className="rail" aria-label="主导航">
        <b className="logo">AG</b>
        <span>☷</span>
        <span>⌂</span>
        <span className="active">▣</span>
        <span>⌁</span>
        <span>♢</span>
        <span>◉</span>
        <span>⚙</span>
        <i />
        <span>?</span>
        <span>⌁</span>
      </nav>
      <div className="workspace">
        <header className="topbar">
          <strong>AgentGuild</strong>
          <button>▣　Billing Platform　⌄</button>
          <span>任务</span>
          <label>⌘ K　 搜索或执行命令…　⌕</label>
          <span className="avatar">👨🏻</span>
          <b>周昊然　⌄</b>
        </header>
        <div className="contextbar">
          <span>⌂　仓库　<b>billing-service</b></span>
          <span>GitLab MR　<b>!284 ↗</b></span>
          <span>Commit　<b>a1b2c3d</b></span>
          <span>Agent　<b>▣ Atlas v12</b></span>
          <span>状态　<b className="warning">● waiting review</b></span>
          <span>耗时　<b>2h37m</b></span>
          <span>成本　<b>$0.142</b></span>
        </div>
        <main>
          <Routes>
            <Route path="/tasks" element={<Workbench />} />
            <Route path="/tasks/:taskId" element={<Workbench />} />
            <Route path="*" element={<Navigate to="/tasks" replace />} />
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

function Workbench() {
  const { taskId } = useParams();
  const navigate = useNavigate();
  return (
    <div className="observer">
      <div className="page-title">
        <div>
          <h1>任务</h1>
          <span>发布、分配并追踪 Agent 的代码任务</span>
        </div>
        <span className="readonly">只读观察模式</span>
      </div>
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
