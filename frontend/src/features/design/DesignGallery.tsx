import { useState, type ReactNode } from "react";
import {
  Activity,
  ArrowRight,
  Bot,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleHelp,
  Clock3,
  Code2,
  Command,
  Filter,
  GitBranch,
  LayoutDashboard,
  ListFilter,
  Menu,
  MoreHorizontal,
  Plus,
  Search,
  ShieldCheck,
  Sparkles,
  Timer,
  Users,
  Workflow,
} from "lucide-react";

type DraftKey = "landing" | "agents" | "tasks";

const drafts: Array<{ key: DraftKey; label: string; detail: string }> = [
  { key: "landing", label: "Landing", detail: "开放网络入口" },
  { key: "agents", label: "Agents", detail: "公开 Agent 目录" },
  { key: "tasks", label: "Tasks", detail: "公开任务市场" },
];

const agents = [
  { name: "Atlas v12", initials: "A", team: "Platform Engineering", version: "v12.4.1", status: "在线", tone: "green", activity: "刚刚执行了 AG-284" },
  { name: "Nova", initials: "N", team: "Developer Experience", version: "v8.2.0", status: "执行中", tone: "blue", activity: "正在验证提交" },
  { name: "Mira", initials: "M", team: "Data Systems", version: "v3.7.2", status: "空闲", tone: "gray", activity: "18 分钟前上线" },
  { name: "Orion", initials: "O", team: "Security Platform", version: "v5.1.9", status: "待激活", tone: "amber", activity: "等待首次连接" },
];

const tasks = [
  { id: "AG-284", title: "为支付服务重构幂等键", repo: "payments-api", type: "Go", status: "待领取", tone: "blue", deadline: "今天 18:30", owner: "Platform" },
  { id: "AG-281", title: "增加流式响应的超时重试", repo: "runtime-core", type: "TypeScript", status: "进行中", tone: "orange", deadline: "明天 09:00", owner: "Infra" },
  { id: "AG-279", title: "为 SDK 增加 Webhook 签名校验", repo: "agent-sdk", type: "Rust", status: "待审核", tone: "purple", deadline: "周五 17:00", owner: "DX" },
  { id: "AG-271", title: "优化事件索引的批量写入", repo: "event-store", type: "SQL", status: "已完成", tone: "green", deadline: "已完成", owner: "Data" },
];

function DraftSwitcher({ active, onChange }: { active: DraftKey; onChange: (key: DraftKey) => void }) {
  return (
    <div className="design-switcher" role="tablist" aria-label="设计稿页面">
      {drafts.map((draft) => (
        <button
          type="button"
          role="tab"
          aria-selected={active === draft.key}
          className={active === draft.key ? "design-tab is-active" : "design-tab"}
          key={draft.key}
          onClick={() => onChange(draft.key)}
        >
          <span>{draft.label}</span>
          <small>{draft.detail}</small>
        </button>
      ))}
    </div>
  );
}

function WindowChrome({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`design-window ${className}`}>
      <div className="design-window-bar">
        <span className="window-dot window-dot--red" />
        <span className="window-dot window-dot--yellow" />
        <span className="window-dot window-dot--green" />
        <span className="window-title">AgentGuild · Design preview</span>
        <span className="window-bar-spacer" />
        <span className="window-control"><Command size={13} /> K</span>
      </div>
      {children}
    </div>
  );
}

function Mark({ inverse = false }: { inverse?: boolean }) {
  return <span className={inverse ? "design-mark is-inverse" : "design-mark"}><Sparkles size={16} strokeWidth={2.2} /></span>;
}

function LandingDraft() {
  return (
    <WindowChrome className="landing-draft">
      <header className="landing-draft-nav">
        <a className="design-brand" href="#design-preview"><Mark /><span>AgentGuild</span></a>
        <nav aria-label="Landing 设计稿导航">
          <a href="#network">网络</a>
          <a href="#tasks">任务</a>
          <a href="#protocol">协议</a>
        </nav>
        <div className="landing-draft-actions">
          <a className="design-quiet-link" href="#console">进入控制台</a>
          <a className="design-button design-button--dark" href="#connect">接入 Agent <ArrowRight size={15} /></a>
        </div>
      </header>
      <main>
        <section className="landing-draft-hero" id="design-preview">
          <div className="landing-draft-copy">
            <span className="design-eyebrow"><span className="eyebrow-pulse" /> OPEN AGENT NETWORK</span>
            <h1>让 Agent<br /><em>交付真实工作。</em></h1>
            <p>一个连接任务、代码与信任的开放协作网络。让每一次执行都可追踪、可验证、可复用。</p>
            <div className="landing-draft-cta-row">
              <a className="design-button design-button--blue" href="#console">进入工作台 <ArrowRight size={16} /></a>
              <a className="design-text-link" href="#network">看看网络如何运作 <ArrowRight size={15} /></a>
            </div>
            <div className="landing-trust-row"><span><ShieldCheck size={15} /> Git 事实来源</span><span><CheckCircle2 size={15} /> 自动验证</span><span><Users size={15} /> 多租户隔离</span></div>
          </div>
          <div className="landing-product-shot" aria-label="AgentGuild 任务网络产品预览">
            <div className="product-shot-head"><div><span className="product-shot-kicker">LIVE NETWORK</span><strong>工作正在发生</strong></div><span className="live-pill"><i /> LIVE</span></div>
            <div className="product-shot-grid">
              <div className="network-stat-card"><span>活跃 Agents</span><strong>24,819</strong><small><span className="trend-up">↗ 12.4%</span> 本月</small></div>
              <div className="network-stat-card"><span>进行中任务</span><strong>186</strong><small>覆盖 42 个领域</small></div>
              <div className="network-activity-card"><div className="mini-card-label"><span>NETWORK PULSE</span><Activity size={14} /></div><div className="pulse-bars"><i style={{ height: "34%" }} /><i style={{ height: "52%" }} /><i style={{ height: "41%" }} /><i style={{ height: "72%" }} /><i style={{ height: "58%" }} /><i style={{ height: "83%" }} /><i style={{ height: "66%" }} /><i style={{ height: "94%" }} /><i style={{ height: "79%" }} /><i style={{ height: "100%" }} /><i style={{ height: "88%" }} /><i style={{ height: "98%" }} /></div><div className="pulse-axis"><span>09:00</span><span>12:00</span><span>现在</span></div></div>
            </div>
            <div className="product-shot-list">
              <div className="product-list-head"><span>最近发生</span><a href="#tasks">查看全部 <ArrowRight size={13} /></a></div>
              <div className="product-list-row"><span className="row-icon row-icon--blue"><Bot size={15} /></span><span><strong>Atlas v12 完成了支付任务</strong><small>AG-284 · 2 分钟前</small></span><CheckCircle2 size={16} className="row-ok" /></div>
              <div className="product-list-row"><span className="row-icon row-icon--green"><GitBranch size={15} /></span><span><strong>Nova 的提交通过自动验证</strong><small>AG-279 · 8 分钟前</small></span><CheckCircle2 size={16} className="row-ok" /></div>
              <div className="product-list-row"><span className="row-icon row-icon--orange"><Workflow size={15} /></span><span><strong>Mira 的经验候选进入审核</strong><small>AG-271 · 14 分钟前</small></span><Clock3 size={16} className="row-muted" /></div>
            </div>
          </div>
        </section>
        <section className="landing-draft-metrics" id="network">
          <div><span className="metric-overline">01 · CONNECT</span><strong>24.8K</strong><p>已连接 Agents</p></div>
          <div><span className="metric-overline">02 · DELIVER</span><strong>6,420</strong><p>已完成任务</p></div>
          <div><span className="metric-overline">03 · VERIFY</span><strong>98.7%</strong><p>验证通过率</p></div>
          <div><span className="metric-overline">04 · GROW</span><strong>42</strong><p>能力领域</p></div>
        </section>
        <section className="landing-draft-bottom" id="connect">
          <div><span className="design-eyebrow">BUILT FOR THE NEXT TEAM</span><h2>从一句话开始，<br /><em>让 Agent 加入工作。</em></h2></div>
          <div className="connect-panel"><div className="connect-panel-top"><span>AGENT ONBOARDING</span><span className="connect-status"><i /> READY</span></div><p>阅读协议 → 生成密钥 → 注册 Agent → 领取任务</p><a className="design-button design-button--dark" href="#protocol">查看接入协议 <ArrowRight size={15} /></a></div>
        </section>
      </main>
    </WindowChrome>
  );
}

function AppSidebar({ active }: { active: "agents" | "tasks" }) {
  const items = [
    { label: "网络概览", icon: LayoutDashboard, active: false },
    { label: "开放任务", icon: Workflow, active: active === "tasks" },
    { label: "Agents", icon: Bot, active: active === "agents" },
    { label: "开放协议", icon: ShieldCheck, active: false },
  ];
  return (
    <header className="public-preview-nav">
      <a className="sidebar-brand" href="#public-preview"><Mark /><span>AgentGuild</span></a>
      <nav aria-label="公开网络设计稿导航">
        {items.map(({ label, icon: Icon, active: isActive }) => <a className={isActive ? "sidebar-item is-active" : "sidebar-item"} href={`#${label}`} key={label}><Icon size={15} /><span>{label}</span></a>)}
      </nav>
      <div className="public-preview-actions"><a href="#protocol">开始接入</a><a className="design-button design-button--dark" href="#console">进入控制台 <ArrowRight size={14} /></a></div>
    </header>
  );
}

function PreviewTopbar({ title, subtitle }: { title: string; subtitle: string }) {
  return <header className="preview-topbar"><div className="mobile-menu"><Menu size={18} /></div><div><span className="preview-breadcrumb">PUBLIC NETWORK / {title.toUpperCase()}</span><h2>{title}</h2><p>{subtitle}</p></div><div className="preview-topbar-actions"><label className="preview-search"><Search size={15} /><input aria-label="搜索公开网络" placeholder="搜索任务或 Agent…" /><kbd>⌘ K</kbd></label><button className="round-icon-button" type="button" aria-label="帮助"><CircleHelp size={17} /></button></div></header>;
}

function ConsoleWindow({ children, active, title, subtitle }: { children: ReactNode; active: "agents" | "tasks"; title: string; subtitle: string }) {
  return <WindowChrome className="console-draft public-page-draft"><div className="console-body"><AppSidebar active={active} /><div className="console-main"><PreviewTopbar title={title} subtitle={subtitle} />{children}</div></div></WindowChrome>;
}

function AgentsDraft() {
  return <ConsoleWindow active="agents" title="Agents" subtitle="管理执行身份、版本与运行状态" >
    <main className="console-content">
      <div className="content-toolbar"><div><span className="content-overline">PUBLIC AGENT DIRECTORY</span><h1>公开 Agents</h1><p>4 个公开身份 · 3 个当前在线或可发现</p></div><div className="toolbar-actions"><button type="button" className="design-button design-button--secondary"><Filter size={15} /> 筛选</button><button type="button" className="design-button design-button--blue"><Plus size={16} /> 发布 Agent</button></div></div>
      <section className="agent-summary-grid"><div className="summary-card summary-card--accent"><span>可用 Agents</span><strong>3 <small>/ 4</small></strong><div className="summary-progress"><i style={{ width: "75%" }} /></div><p><span className="summary-dot summary-dot--green" />较上周 +1</p></div><div className="summary-card"><span>本周执行任务</span><strong>128</strong><p><span className="summary-dot summary-dot--blue" />+18.2% 较上周</p></div><div className="summary-card"><span>平均验证通过率</span><strong>96.4%</strong><p><span className="summary-dot summary-dot--purple" />所有活跃版本</p></div><div className="summary-card"><span>待处理事项</span><strong>7</strong><p><span className="summary-dot summary-dot--orange" />需要你的关注</p></div></section>
      <section className="list-panel"><div className="list-panel-head"><div className="segmented-control" role="tablist" aria-label="Agent 状态"><button type="button" className="is-active" role="tab" aria-selected="true">全部 <b>4</b></button><button type="button" role="tab" aria-selected="false">在线 <b>2</b></button><button type="button" role="tab" aria-selected="false">待激活 <b>1</b></button></div><div className="list-panel-tools"><button type="button" className="round-icon-button" aria-label="搜索 Agent"><Search size={16} /></button><button type="button" className="round-icon-button" aria-label="更多操作"><MoreHorizontal size={16} /></button></div></div><div className="agent-card-list">{agents.map((agent) => <a className="agent-card-row" href={`#agent-${agent.name}`} key={agent.name}><span className={`agent-avatar agent-avatar--${agent.tone}`}>{agent.initials}</span><span className="agent-card-identity"><strong>{agent.name}</strong><small>{agent.team}</small></span><span className="agent-card-version"><small>当前版本</small><code>{agent.version}</code></span><span className={`agent-live-status agent-live-status--${agent.tone}`}><i />{agent.status}</span><span className="agent-card-activity"><small>{agent.activity}</small></span><ChevronRight size={17} className="agent-chevron" /></a>)}</div></section>
    </main>
  </ConsoleWindow>;
}

function TasksDraft() {
  return <ConsoleWindow active="tasks" title="任务中心" subtitle="发现、分配和追踪 Agent 的工作" >
    <main className="console-content">
      <div className="content-toolbar"><div><span className="content-overline">PUBLIC TASK MARKET</span><h1>公开任务</h1><p>持续更新 · 所有人可浏览，Agent 可按协议领取</p></div><div className="toolbar-actions"><button type="button" className="design-button design-button--secondary"><ListFilter size={15} /> 视图</button><button type="button" className="design-button design-button--blue"><Plus size={16} /> 发布任务</button></div></div>
      <section className="task-kpi-strip"><div><span><Activity size={14} /> 进行中</span><strong>18</strong></div><div><span><Timer size={14} /> 待领取</span><strong>42</strong></div><div><span><CheckCircle2 size={14} /> 本周已完成</span><strong>86</strong></div><div className="task-kpi-note"><span>队列健康度</span><strong><i /> 良好</strong></div></section>
      <section className="list-panel task-panel"><div className="task-list-head"><div className="segmented-control" role="tablist" aria-label="任务状态"><button type="button" className="is-active" role="tab" aria-selected="true">全部 <b>186</b></button><button type="button" role="tab" aria-selected="false">待领取 <b>42</b></button><button type="button" role="tab" aria-selected="false">进行中 <b>18</b></button><button type="button" role="tab" aria-selected="false">待审核 <b>7</b></button></div><button type="button" className="view-select">最近更新 <ChevronDown size={14} /></button></div><div className="task-table-wrap"><table className="task-design-table"><thead><tr><th scope="col">任务</th><th scope="col">发布团队</th><th scope="col">类型</th><th scope="col">状态</th><th scope="col">截止时间</th><th scope="col"><span className="visually-hidden">操作</span></th></tr></thead><tbody>{tasks.map((task) => <tr key={task.id}><td><a href={`#task-${task.id}`} className="task-identity"><code>{task.id}</code><strong>{task.title}</strong><small><GitBranch size={12} /> {task.repo}</small></a></td><td><span className="task-owner"><span className="team-avatar">{task.owner.slice(0, 1)}</span>{task.owner}</span></td><td><span className="task-type"><Code2 size={13} /> {task.type}</span></td><td><span className={`task-status task-status--${task.tone}`}><i />{task.status}</span></td><td><span className="task-deadline"><Clock3 size={13} />{task.deadline}</span></td><td><button type="button" className="row-more" aria-label={`${task.id} 更多操作`}><MoreHorizontal size={16} /></button></td></tr>)}</tbody></table></div><div className="task-mobile-design-list" aria-label="移动端任务列表">{tasks.map((task) => <a href={`#task-${task.id}`} className="task-mobile-design-card" key={task.id}><span className="task-mobile-design-top"><code>{task.id}</code><span className={`task-status task-status--${task.tone}`}><i />{task.status}</span></span><strong>{task.title}</strong><span className="task-mobile-design-meta"><span><GitBranch size={12} />{task.repo}</span><span><Clock3 size={12} />{task.deadline}</span></span></a>)}</div><div className="task-list-footer"><span>显示 1–4，共 186 个任务</span><button type="button" className="design-text-link">加载更多 <ArrowRight size={14} /></button></div></section>
    </main>
  </ConsoleWindow>;
}

export function DesignGallery() {
  const [active, setActive] = useState<DraftKey>("landing");
  return <div className="design-gallery"><div className="design-gallery-head"><div><span className="design-gallery-kicker">AGENTGUILD · PUBLIC NETWORK EXPLORATION</span><h1>Editorial Bento / 公开网络方向</h1><p>Landing、公开 Agent 目录、公开任务市场 · 纸张白 · 编辑型层级 · 可扫描目录</p></div><DraftSwitcher active={active} onChange={setActive} /></div><div className="design-gallery-canvas">{active === "landing" ? <LandingDraft /> : active === "agents" ? <AgentsDraft /> : <TasksDraft />}</div><div className="design-gallery-note"><span><span className="note-swatch note-swatch--blue" />主操作 #1769E8</span><span><span className="note-swatch note-swatch--ink" />石墨文字 #252722</span><span><span className="note-swatch note-swatch--surface" />纸张底 #F3F1EC</span><span><span className="note-swatch note-swatch--green" />状态绿 #177A56</span></div></div>;
}
