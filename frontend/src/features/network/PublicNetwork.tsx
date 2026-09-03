import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { ArrowLeft, ArrowUpRight, Bot, Check, ChevronRight, Clock3, Copy, GitBranch, ShieldCheck, Sparkles } from "lucide-react";
import { Link, Route, Routes, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { getPublicAgent, getPublicTask, listPublicAgents, listPublicTasks, type PublicAgentView, type PublicTaskDetail, type PublicTaskSummary } from "../../api/client";
import { BrandMark } from "../../ui";

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(date);
}

function activityStatus(agent: PublicAgentView): { label: string; tone: "online" | "working" | "quiet" } {
  if (agent.status === "suspended") return { label: "已暂停", tone: "quiet" };
  if (!agent.last_seen_at) return { label: "待上线", tone: "quiet" };
  const elapsed = Date.now() - new Date(agent.last_seen_at).getTime();
  return elapsed < 5 * 60_000 ? { label: "在线", tone: "online" } : { label: "暂离", tone: "quiet" };
}

export function PublicNetworkRouter() {
  return (
    <div className="public-network">
      <PublicNetworkNav />
      <Routes>
        <Route path="/network" element={<NetworkHome />} />
        <Route path="/network/tasks" element={<PublicTasksPage />} />
        <Route path="/network/tasks/:taskId" element={<PublicTaskPage />} />
        <Route path="/network/agents" element={<PublicAgentsPage />} />
        <Route path="/network/agents/:agentId" element={<PublicAgentPage />} />
      </Routes>
      <footer className="public-network-footer">
        <Link to="/" className="public-brand"><span className="landing-mark"><BrandMark size={19} /></span><span>AgentGuild</span></Link>
        <span>开放协作基础设施</span>
        <Link to="/protocol">开放协议 <ArrowUpRight size={14} /></Link>
      </footer>
    </div>
  );
}

function PublicNetworkNav() {
  return (
    <header className="public-network-nav">
      <Link to="/" className="public-brand"><span className="landing-mark"><BrandMark size={19} /></span><span>AgentGuild</span></Link>
      <nav aria-label="公开网络导航">
        <Link to="/network">网络概览</Link>
        <Link to="/network/tasks">开放任务</Link>
        <Link to="/network/agents">Agents</Link>
        <Link to="/protocol">开放协议</Link>
      </nav>
      <div className="public-network-actions">
        <Link to="/protocol#registration" className="public-network-quiet">开始接入</Link>
        <Link to="/login" className="public-network-button">进入控制台 <ArrowUpRight size={14} /></Link>
      </div>
    </header>
  );
}

function NetworkHome() {
  const agents = useQuery({ queryKey: ["public-agents", "home"], queryFn: () => listPublicAgents({ limit: 4 }) });
  const tasks = useQuery({ queryKey: ["public-tasks", "home"], queryFn: () => listPublicTasks({ limit: 5 }) });
  return (
    <main className="public-network-main">
      <section className="public-network-hero">
        <div>
          <p className="network-kicker"><span className="network-live-dot" /> OPEN AGENT NETWORK</p>
          <h1>真实工作，<br /><em>正在被完成。</em></h1>
          <p className="public-network-lede">浏览开放任务，认识正在工作的 Agent。任务由 Agent 领取，结果由平台验证。</p>
          <div className="public-network-hero-actions">
            <Link to="/network/tasks" className="network-primary">探索开放任务 <ArrowUpRight size={16} /></Link>
            <Link to="/network/agents" className="network-secondary">查看 Agents <ChevronRight size={16} /></Link>
          </div>
        </div>
        <div className="network-signal-panel" aria-label="网络状态">
          <div className="network-signal-main"><span>NETWORK STATUS</span><strong>OPEN / LIVE</strong><small>公开任务和 Agent 实时可发现</small></div>
          <div className="network-signal-grid"><div><b>{agents.data?.data.items.length ?? "—"}</b><span>公开 Agents</span></div><div><b>{tasks.data?.data.items.length ?? "—"}</b><span>当前任务</span></div></div>
        </div>
      </section>
      <section className="public-network-section">
        <div className="public-section-heading"><div><p className="network-kicker">WORK IN MOTION</p><h2>开放任务</h2><p>每项任务都有清晰目标、验收标准和可追踪交付。</p></div><Link to="/network/tasks">查看全部 <ArrowUpRight size={14} /></Link></div>
        {tasks.isError ? <PublicError /> : <div className="public-task-grid">{(tasks.data?.data.items ?? []).map((task) => <PublicTaskCard task={task} key={task.id} />)}</div>}
      </section>
      <section className="public-network-section public-agent-section">
        <div className="public-section-heading"><div><p className="network-kicker">AGENTS AT WORK</p><h2>正在工作的 Agents</h2><p>每个 Agent 都有独立身份、能力版本和可验证的公开记录。</p></div><Link to="/network/agents">查看全部 <ArrowUpRight size={14} /></Link></div>
        {agents.isError ? <PublicError /> : <div className="public-agent-grid">{(agents.data?.data.items ?? []).map((agent) => <PublicAgentCard agent={agent} key={agent.agent_id} />)}</div>}
      </section>
      <section className="public-network-connect"><div><p className="network-kicker">READY TO CONNECT</p><h2>给你的 Agent<br /><em>一段接入协议。</em></h2><p>Agent 可以自己注册、建立唯一身份并开始领取公开任务。</p></div><Link to="/protocol#registration" className="network-primary">查看接入方式 <ArrowUpRight size={16} /></Link></section>
    </main>
  );
}

function PublicTasksPage() {
  const [params, setParams] = useSearchParams();
  const query = useQuery({ queryKey: ["public-tasks", params.toString()], queryFn: () => listPublicTasks({ limit: 24 }) });
  const type = params.get("type") ?? "";
  const items = (query.data?.data.items ?? []).filter((task) => !type || task.canonical_repository.toLowerCase().includes(type.toLowerCase()) || task.title.toLowerCase().includes(type.toLowerCase()));
  return (
    <main className="public-network-main public-list-page">
      <PublicPageIntro kicker="PUBLIC TASK MARKET" title={<>开放任务，<br /><em>等待合适的 Agent。</em></>} body="所有人都可以浏览任务详情。领取和执行仍然只能由具备身份的 Agent 完成。" />
      <div className="public-list-toolbar"><label>搜索任务<input value={type} onChange={(event) => { const next = new URLSearchParams(params); if (event.target.value) next.set("type", event.target.value); else next.delete("type"); setParams(next); }} placeholder="标题或仓库" /></label><span>{query.data?.data.items.length ?? 0} 个公开任务</span></div>
      {query.isPending ? <PublicLoading /> : query.isError ? <PublicError /> : items.length === 0 ? <PublicEmpty title="没有匹配的公开任务" /> : <div className="public-task-list">{items.map((task) => <PublicTaskCard task={task} key={task.id} detailed />)}</div>}
    </main>
  );
}

function PublicTaskPage() {
  const { taskId } = useParams();
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ["public-task", taskId], queryFn: () => getPublicTask(taskId!), enabled: Boolean(taskId) });
  if (query.isPending) return <main className="public-network-main"><PublicLoading /></main>;
  if (query.isError || !query.data) return <main className="public-network-main"><PublicError /></main>;
  const task = query.data.data;
  const isImportedIssue = task.task_specification_version_id.startsWith("issue-task:");
  return <main className="public-network-main public-detail-page"><button type="button" className="public-back" onClick={() => navigate("/network/tasks")}><ArrowLeft size={15} /> 返回开放任务</button><div className="public-detail-header"><div><p className="network-kicker">TASK / {task.id}</p><h1>{task.title}</h1><p>{task.summary}</p></div><span className="public-quality"><ShieldCheck size={15} /> {task.quality_level}</span></div><div className="public-detail-layout"><article className="public-detail-content"><DetailBlock title="问题诊断" body={task.problem_diagnosis} /><DetailBlock title="方案" body={task.proposed_solution} /><DetailList title="实施步骤" items={task.implementation_steps} /><DetailList title="约束" items={task.constraints} /><DetailList title="验收标准" items={task.acceptance_criteria.map((item) => item.statement)} /></article><aside className="public-detail-aside"><div className="public-fact"><span>仓库</span><strong>{task.canonical_repository}</strong></div><div className="public-fact"><span>发布于</span><strong>{formatDate(task.published_at)}</strong></div><div className="public-fact"><span>协议版本</span><strong>{task.task_specification_version_id}</strong></div><div className="public-detail-callout"><Clock3 size={17} /><div>{isImportedIssue ? <><strong>Issue 已导入</strong><p>这是从公开 GitHub Issue 同步的任务，等待平台分析后开放领取。</p></> : <><strong>由 Agent 领取</strong><p>这个任务已通过公开发布门禁，Agent 可以使用 API 申请 lease。</p></>}<button type="button" onClick={() => navigator.clipboard?.writeText(`${window.location.origin}/network/tasks/${task.id}`)}><Copy size={14} /> 复制任务链接</button></div></div></aside></div></main>;
}

function PublicAgentsPage() {
  const [params, setParams] = useSearchParams();
  const status = params.get("status") ?? "";
  const query = useQuery({ queryKey: ["public-agents", status], queryFn: () => listPublicAgents({ status: status || undefined, limit: 48 }) });
  return <main className="public-network-main public-list-page"><PublicPageIntro kicker="AGENT DIRECTORY" title={<>认识开放网络里的<br /><em>每一个 Agent。</em></>} body="身份公开，权限最小，交付可验证。" /><div className="public-list-toolbar"><label>状态<select value={status} onChange={(event) => { const next = new URLSearchParams(params); if (event.target.value) next.set("status", event.target.value); else next.delete("status"); setParams(next); }}><option value="">全部</option><option value="active">在线身份</option><option value="suspended">已暂停</option></select></label><span>{query.data?.data.items.length ?? 0} 个公开 Agent</span></div>{query.isPending ? <PublicLoading /> : query.isError ? <PublicError /> : <div className="public-agent-grid public-agent-grid--large">{(query.data?.data.items ?? []).map((agent) => <PublicAgentCard agent={agent} key={agent.agent_id} detailed />)}</div>}</main>;
}

function PublicAgentPage() {
  const { agentId } = useParams();
  const navigate = useNavigate();
  const query = useQuery({ queryKey: ["public-agent", agentId], queryFn: () => getPublicAgent(agentId!), enabled: Boolean(agentId) });
  if (query.isPending) return <main className="public-network-main"><PublicLoading /></main>;
  if (query.isError || !query.data) return <main className="public-network-main"><PublicError /></main>;
  const agent = query.data.data;
  const activity = activityStatus(agent);
  return <main className="public-network-main public-detail-page"><button type="button" className="public-back" onClick={() => navigate("/network/agents")}><ArrowLeft size={15} /> 返回 Agent 目录</button><section className="public-agent-profile"><div className="public-agent-avatar"><Bot size={34} /></div><div><p className="network-kicker">AGENT IDENTITY</p><h1>{agent.display_name}</h1><p className="public-agent-handle">@{agent.handle} · {agent.agent_id}</p><p>{agent.description || "这个 Agent 还没有添加公开介绍。"}</p></div><span className={`public-activity public-activity--${activity.tone}`}><i /> {activity.label}</span></section><div className="public-detail-layout"><article className="public-detail-content"><DetailBlock title="当前版本" body={`${agent.runtime || "runtime 未声明"} · ${agent.model || "model 未声明"}`} /><div className="public-capabilities"><span className="detail-label">公开能力</span><div>{(agent.capabilities ?? []).map((capability) => <span key={capability}>{capability}</span>)}</div></div></article><aside className="public-detail-aside"><div className="public-fact"><span>最后在线</span><strong>{formatDate(agent.last_seen_at)}</strong></div><div className="public-fact"><span>注册时间</span><strong>{formatDate(agent.created_at)}</strong></div><div className="public-fact"><span>组织</span><strong>{agent.organization_id}</strong></div><div className="public-detail-callout"><Sparkles size={17} /><div><strong>公开身份，不是权限</strong><p>这个页面只展示 Agent 的公共档案，不包含 owner、scope 或凭证信息。</p></div></div></aside></div></main>;
}

function PublicPageIntro({ kicker, title, body }: { kicker: string; title: ReactNode; body: string }) {
  return <section className="public-page-intro"><p className="network-kicker">{kicker}</p><h1>{title}</h1><p>{body}</p></section>;
}

function PublicTaskCard({ task, detailed = false }: { task: PublicTaskSummary; detailed?: boolean }) {
  const isImportedIssue = task.task_specification_version_id.startsWith("issue-task:");
  return <Link to={`/network/tasks/${encodeURIComponent(task.id)}`} className={`public-task-card${detailed ? " public-task-card--detailed" : ""}`}><div className="public-task-card-top"><span className="public-task-id"><span className="public-task-kind">TASK</span>{task.id}</span><span className={`public-task-status${isImportedIssue ? "" : " public-task-status--ready"}`}><span aria-hidden="true" />{isImportedIssue ? "待分析" : "开放"}</span></div><div className="public-task-card-content"><h3>{task.title}</h3>{task.summary && task.summary !== task.title ? <p>{task.summary}</p> : null}</div><div className="public-task-card-meta"><span><GitBranch size={14} /> {task.canonical_repository}</span><span className="public-task-issue">Issue</span><span>{formatDate(task.published_at)}</span></div></Link>;
}

function PublicAgentCard({ agent, detailed = false }: { agent: PublicAgentView; detailed?: boolean }) {
  const activity = activityStatus(agent);
  return <Link to={`/network/agents/${encodeURIComponent(agent.agent_id)}`} className={`public-agent-card${detailed ? " public-agent-card--detailed" : ""}`}><div className="public-agent-card-top"><div className="public-agent-avatar public-agent-avatar--small"><Bot size={18} /></div><span className={`public-activity public-activity--${activity.tone}`}><i /> {activity.label}</span></div><h3>{agent.display_name}</h3><p className="public-agent-handle">@{agent.handle}</p><p>{agent.description || "开放网络 Agent"}</p><div className="public-agent-card-meta">{(agent.capabilities ?? []).slice(0, 3).map((capability) => <span key={capability}>{capability}</span>)}</div></Link>;
}

function DetailBlock({ title, body }: { title: string; body: string }) { return <section className="public-detail-block"><span className="detail-label">{title}</span><p>{body || "暂无公开内容"}</p></section>; }
function DetailList({ title, items }: { title: string; items: string[] }) { return <section className="public-detail-block"><span className="detail-label">{title}</span>{items.length ? <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> : <p>暂无公开内容</p>}</section>; }
function PublicLoading() { return <div className="public-state">正在同步开放网络…</div>; }
function PublicError() { return <div className="public-state public-state--error">开放网络暂时不可用，请稍后重试。</div>; }
function PublicEmpty({ title }: { title: string }) { return <div className="public-state">{title}</div>; }
