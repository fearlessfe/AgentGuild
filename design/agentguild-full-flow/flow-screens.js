// The ten canonical flow screens. Each returns the inner HTML of a `.screen`
// article. Human UI never exposes Agent-only claim/execute/submit/credential
// operations; API feasibility lives only in `apiNote` annotations.

import {
  appShell,
  apiNote,
  button,
  card,
  checkRow,
  denseTable,
  diffViewer,
  escapeHtml,
  filterBar,
  metricGrid,
  pageHeader,
  providerCard,
  rubricForm,
  statusChip,
  steps,
  tabs,
  timeline,
} from "./components.js";
import { sharedData } from "./screens.js";

const ONBOARDING_STEPS = [
  { title: "登录", state: "done" },
  { title: "Git 接入", state: "current", desc: "连接企业 Git 提供商并授予最小权限" },
  { title: "仓库", state: "pending" },
  { title: "同步", state: "pending" },
  { title: "完成", state: "pending" },
];

// -------------------------------------------------------------- 01 login

function login() {
  return `
    <div class="auth-screen">
      <div class="auth-brand">
        <span class="rail-brand" aria-hidden="true">AG</span>
        <div>
          <div class="auth-title">AgentGuild</div>
          <div class="auth-tagline">企业 Agent 任务治理平台</div>
        </div>
      </div>
      <div class="auth-card">
        ${card({
          title: "登录",
          sub: "使用企业身份登录以进入治理工作台",
          body: `
            <div class="stack">
              ${button("使用企业 OIDC 登录", { variant: "primary", block: true, lg: true, icon: "⛨" })}
              <div class="auth-divider"><span>本地开发登录</span></div>
              <div class="field">
                <label class="field-label">用户名</label>
                <div class="input">admin</div>
              </div>
              <div class="field">
                <label class="field-label">密码</label>
                <div class="input input--focus">••••••••</div>
              </div>
              <div class="auth-error" role="alert">密码错误或未启用本地登录</div>
              ${button("本地登录", { block: true })}
              <p class="field-hint">本地登录仅在服务端启用时显示。</p>
            </div>`,
        })}
        ${apiNote("available", "OIDC 登录/回调与 local login 路由均存在；local login 仍需补入 OpenAPI。")}
      </div>
    </div>`;
}

// --------------------------------------------------------- 02 onboarding

function onboarding() {
  const body = `
    <div class="stack">
      ${pageHeader({
        title: "首次引导",
        sub: "完成四步接入后即可开始任务同步。成功同步一次才视为引导完成。",
      })}
      <div class="split-2">
        <div class="col">
          ${card({
            title: "接入 Git 提供商",
            sub: "当前步骤 · Git 接入",
            body: `
              <div class="stack">
                <p class="muted text-sm">选择企业 Git 提供商并安装 AgentGuild App，仅授予任务治理所需的最小权限。</p>
                <div class="row">
                  ${button("连接 GitHub", { variant: "primary", icon: "" })}
                  ${button("稍后再说", { variant: "ghost" })}
                </div>
              </div>`,
          })}
          ${apiNote("planned", "服务端尚无 onboarding 状态接口；完成状态必须由服务端持久化，不能用前端本地状态代替。")}
        </div>
        <div class="col">
          ${card({ title: "引导进度", body: steps(ONBOARDING_STEPS) })}
          <div class="row-between">
            ${button("保存并继续", { variant: "primary" })}
            <span class="faint text-xs">进度已保存</span>
          </div>
        </div>
      </div>
    </div>`;
  return appShell({ module: "首次引导", body });
}

// ------------------------------------------------------ 03 git-integration

function gitIntegration() {
  const githubFooter = `
    <div class="stack-sm">
      <div class="row-between">
        <span class="card-sub">状态</span>
        ${statusChip("已配置", "success")}
      </div>
      <div class="field">
        <label class="field-label">GitHub App 私钥</label>
        <div class="input"><span class="faint">已保存 · 不可读取</span></div>
        <p class="field-hint">出于安全，已存储的私钥永不回显。</p>
      </div>
      <div class="row">
        ${button("检测连接", { icon: "⤿" })}
        ${button("更新配置", {})}
        ${button("删除", { variant: "danger" })}
      </div>
    </div>`;

  const body = `
    <div class="stack">
      ${pageHeader({ title: "Git 接入", sub: "使用通用 Git 提供商外壳；当前仅启用 GitHub。" })}
      <div class="split-2">
        <div class="col">
          ${providerCard({
            logo: "◐",
            name: "GitHub",
            note: "已连接 · Atlas Billing 组织",
            permissions: ["Contents — 只读", "Issues — 读写", "Checks — 只读", "Metadata — 只读"],
            footer: githubFooter,
          })}
        </div>
        <div class="col">
          ${providerCard({
            logo: "◑",
            name: "GitLab",
            note: "即将支持",
            disabled: true,
            footer: `<div class="row-between"><span class="card-sub">状态</span>${statusChip("即将支持", "neutral")}</div>`,
          })}
          ${apiNote("available", "GET/POST/DELETE /v1/github-app 已提供 CRUD；前端仍待接入。")}
          ${apiNote("planned", "连接检测（connection test）动作需要新增后端能力。")}
        </div>
      </div>
    </div>`;
  return appShell({ module: "Git 接入", active: "同步", body });
}

// -------------------------------------------------- 04 repository-sync-rule

function repositorySyncRule() {
  const repoRows = [
    { cells: [{ text: "billing-service" }, { html: statusChip("已启用", "success") }, { html: "<code>main</code>" }, { text: "private" }], selected: true },
    { cells: [{ text: "event-gateway" }, { html: statusChip("已启用", "success") }, { html: "<code>main</code>" }, { text: "private" }] },
    { cells: [{ text: "legacy-monolith" }, { html: statusChip("已停用", "neutral") }, { html: "<code>master</code>" }, { text: "private" }] },
  ];

  const rule = [
    ["包含标签", "agent-ready"],
    ["排除标签", "security-hold, needs-product"],
    ["Issue 状态", "open"],
    ["任务类型", "code"],
    ["默认优先级", "P1"],
    ["同步频率", "每 5 分钟"],
    ["重复策略", "更新现有 Task"],
  ]
    .map(
      ([k, v]) => `
        <div class="rule-field">
          <span class="field-label">${escapeHtml(k)}</span>
          <div class="input">${escapeHtml(v)}</div>
        </div>`,
    )
    .join("");

  const body = `
    <div class="stack">
      ${pageHeader({
        title: "仓库与同步规则",
        sub: "选择要纳入治理的仓库，并用规则驱动 Issue → Task 同步。",
        actions: `${button("保存草稿", {})}${button("预览", { icon: "⤿" })}${button("启用规则", { variant: "primary" })}`,
      })}
      <div class="split-2">
        <div class="col col--fill">
          ${card({
            title: "安装仓库",
            sub: "3 个仓库",
            pad: false,
            body: denseTable({
              columns: ["仓库", "同步", "默认分支", "可见性"],
              rows: repoRows,
              caption: "安装仓库列表",
            }),
          })}
        </div>
        <div class="col">
          ${card({ title: "同步规则", body: `<div class="rule-grid">${rule}</div>` })}
          ${card({
            title: "自然语言预览",
            body: `<p class="muted text-sm">每 5 分钟同步 <strong>billing-service</strong> 中带 <code>agent-ready</code>、状态为 open 的 Issue，排除 <code>security-hold</code> 与 <code>needs-product</code>，生成 P1 的 code 任务；已存在的 Task 将被更新。</p>`,
          })}
          <div class="row-between">
            ${button("暂停规则", { variant: "ghost" })}
            <span class="faint text-xs">影响：预计新增 8 个任务</span>
          </div>
          ${apiNote("planned", "仓库目录与同步规则 CRUD/预览/启停均为规划能力。")}
        </div>
      </div>
    </div>`;
  return appShell({ module: "仓库与规则", active: "同步", body });
}

// ------------------------------------------------------------ 05 sync-result

function syncResult() {
  const rows = [
    { cells: [{ html: "<code>#412</code>" }, { html: `<code>${sharedData.taskId}</code>` }, { html: statusChip("新增", "success") }, { text: "标签匹配 agent-ready" }, { text: "20s 前" }, { html: button("查看", { variant: "ghost" }) }] },
    { cells: [{ html: "<code>#398</code>" }, { html: "<code>AG-188</code>" }, { html: statusChip("更新", "info") }, { text: "标题与验收条件变更" }, { text: "20s 前" }, { html: button("查看", { variant: "ghost" }) }] },
    { cells: [{ html: "<code>#377</code>" }, { text: "—" }, { html: statusChip("忽略", "neutral") }, { text: "命中排除标签 needs-product" }, { text: "20s 前" }, { html: button("查看", { variant: "ghost" }) }] },
    { cells: [{ html: "<code>#365</code>" }, { html: "<code>AG-171</code>" }, { html: statusChip("冲突", "warning") }, { text: "Task 已被人工修改" }, { text: "21s 前" }, { html: `${button("重试", { variant: "ghost" })}${button("忽略", { variant: "ghost" })}${button("暂停规则", { variant: "ghost" })}` }] },
    { cells: [{ html: "<code>#359</code>" }, { text: "—" }, { html: statusChip("失败", "danger") }, { text: "GitHub 速率限制 · 可重试" }, { text: "21s 前" }, { html: button("重试", { variant: "ghost" }) }] },
  ];

  const body = `
    <div class="stack">
      ${pageHeader({ title: "同步结果", sub: "规则 billing-service · agent-ready 的最近一次运行。" })}
      ${metricGrid([
        { label: "新增", value: "8", positive: true },
        { label: "更新", value: "3" },
        { label: "忽略", value: "5" },
        { label: "冲突", value: "2" },
      ])}
      <div class="row"><span class="faint text-sm">另有</span>${statusChip("失败 1", "danger")}<span class="faint text-sm">为平台可重试错误</span></div>
      ${card({
        title: "运行明细",
        sub: "19 条",
        pad: false,
        body: denseTable({
          columns: ["Issue", "Task", "结果", "原因", "更新时间", "操作"],
          rows,
          caption: "同步运行明细",
        }),
      })}
      ${apiNote("planned", "同步运行、结果列表、冲突处理与重试/审计均为规划能力；冲突行不提供直接编辑正式 Task 的入口。")}
    </div>`;
  return appShell({ module: "同步结果", active: "同步", body });
}

// ------------------------------------------------------------ 06 task-center

function taskCenter() {
  const rows = [
    { group: "进行中 · 3" },
    { cells: [{ html: `<code>${sharedData.taskId}</code>` }, { text: "结算重试幂等键缺失" }, { text: "billing-service" }, { text: "code" }, { text: "Atlas v12" }, { html: statusChip("进行中", "info") }, { text: "今天 18:00" }, { text: "Issue #412" }], selected: true },
    { cells: [{ html: "<code>AG-188</code>" }, { text: "事件网关重复投递" }, { text: "event-gateway" }, { text: "code" }, { text: "Atlas v12" }, { html: statusChip("进行中", "info") }, { text: "明天 12:00" }, { text: "Issue #398" }] },
    { cells: [{ html: "<code>AG-171</code>" }, { text: "对账报表时区错误" }, { text: "billing-service" }, { text: "code" }, { text: "—" }, { html: statusChip("待审核", "warning") }, { text: "—" }, { text: "Issue #365" }] },
    { group: "待领取 · 5" },
    { cells: [{ html: "<code>AG-201</code>" }, { text: "退款回调签名校验" }, { text: "billing-service" }, { text: "code" }, { text: "—" }, { html: statusChip("待领取", "neutral") }, { text: "本周五" }, { text: "Issue #420" }] },
  ];

  const detail = `
    <div class="stack">
      <div class="row-between"><strong>${escapeHtml(sharedData.taskId)}</strong>${statusChip("进行中", "info")}</div>
      <div class="detail-block"><span class="ctx-label">来源 Issue</span><div class="text-sm"><code>billing-service#412</code> · 结算重试幂等键缺失</div></div>
      <div class="detail-block"><span class="ctx-label">目标</span><div class="text-sm">为结算重试补充幂等键，避免重复扣款。</div></div>
      <div class="detail-block"><span class="ctx-label">验收条件</span>
        <ul class="perm-list">
          <li>重复请求返回同一结果</li>
          <li>新增幂等键迁移与索引</li>
          <li>公共测试全部通过</li>
        </ul>
      </div>
      <div class="detail-block"><span class="ctx-label">活动执行</span><div class="text-sm">Atlas v12 · lease 剩余 1h42m</div></div>
      <div class="detail-block"><span class="ctx-label">最近同步</span><div class="text-sm faint">20 秒前 · 无冲突</div></div>
      ${apiNote("partial", "来源 Issue、同步批次与同步状态字段需扩展。")}
    </div>`;

  const body = `
    <div class="stack">
      ${pageHeader({ title: "任务中心", sub: "任务由同步规则生成；人工不领取、不提交，仅治理与审核。" })}
      ${tabs([
        { label: "全部", active: true },
        { label: "待领取" },
        { label: "进行中" },
        { label: "待审核" },
        { label: "已完成" },
        { label: "异常" },
      ])}
      ${filterBar([
        { label: "仓库", value: "billing-service" },
        { label: "类型", value: "code" },
        { label: "Agent", value: "全部" },
        { label: "来源", value: "Issue 同步" },
        { label: "同步状态", value: "正常" },
        { label: "时间", value: "近 7 天" },
      ])}
      <div class="split-2 split-2--wide">
        <div class="col col--fill">
          ${card({
            pad: false,
            body: denseTable({
              columns: ["Task ID", "标题", "仓库", "类型", "Agent", "状态", "截止时间", "来源"],
              rows,
              caption: "任务列表",
            }),
          })}
          ${apiNote("available", "GET /v1/tasks · GET /v1/tasks/{id} 支撑任务核心字段。")}
        </div>
        <div class="col">${card({ body: detail })}</div>
      </div>
    </div>`;
  return appShell({ module: "任务中心", active: "任务中心", body });
}

// ------------------------------------------------------- 07 execution-detail

function executionDetail() {
  const events = [
    { time: "17:02:10", title: "任务已领取", note: "Atlas v12 获得 lease" },
    { time: "17:02:14", title: "执行已开始" },
    { time: "17:02:15", title: "凭证已签发", note: "短期 Git 令牌 · 平台代理" },
    { time: "17:08:41", title: "运行测试", note: "公共测试 42/42" },
    { time: "17:12:03", title: "Commit 已推送", note: `${sharedData.commit} → ${sharedData.branch}` },
    { time: "17:12:05", title: "等待提交验证" },
  ];

  const body = `
    <div class="stack">
      ${pageHeader({ title: "执行详情", sub: `${sharedData.taskId} · 由 Atlas v12 执行` })}
      <div class="split-2">
        <div class="col">
          ${card({
            title: "执行事实",
            body: `
              <div class="fact-grid">
                <div class="fact"><span class="ctx-label">Agent</span><span>Atlas v12</span></div>
                <div class="fact"><span class="ctx-label">Lease</span><span>剩余 1h 42m</span></div>
                <div class="fact"><span class="ctx-label">最近心跳</span><span>20 秒前</span></div>
                <div class="fact"><span class="ctx-label">分支</span><code>${escapeHtml(sharedData.branch)}</code></div>
                <div class="fact"><span class="ctx-label">Base</span><code>${escapeHtml(sharedData.baseCommit)}</code></div>
                <div class="fact"><span class="ctx-label">观测成本</span><span>$0.067</span></div>
                <div class="fact"><span class="ctx-label">自报成本</span><span>$0.070</span></div>
                <div class="fact"><span class="ctx-label">覆盖率</span>${statusChip("partial", "warning")}</div>
              </div>
              <p class="field-hint">不以 progress 百分比表示完成度，改用阶段与事件事实。</p>`,
          })}
          ${apiNote("partial", "GET /v1/executions/{id} 提供 lease/状态/阶段/成本；完整事件与 revision 列表需新增查询。")}
        </div>
        <div class="col">
          ${card({ title: "事件时间线", body: timeline(events) })}
        </div>
      </div>
    </div>`;
  return appShell({ module: "执行详情", active: "任务中心", body });
}

// ------------------------------------------------ 08 submission-validation

function submissionValidation() {
  const checks = [
    { name: "Commit 存在", state: "pass", meta: `${sharedData.commit} 可达` },
    { name: "分支匹配", state: "pass", meta: sharedData.branch },
    { name: "Base 为祖先", state: "pass", meta: `${sharedData.baseCommit} 是祖先` },
    { name: "构建", state: "pass" },
    { name: "公共测试", state: "pass", value: "42/42" },
    { name: "隐藏测试", state: "warn", value: "18/20" },
    { name: "安全扫描", state: "pass" },
    { name: "变更范围", state: "warn", meta: "触及结算核心路径" },
  ];

  const body = `
    <div class="stack">
      ${pageHeader({ title: "提交与验证", sub: `${sharedData.taskId} · 验证 attempt 1`, actions: button("平台重试", { icon: "⤿" }) })}
      <div class="split-2">
        <div class="col">
          ${card({
            title: "提交信息",
            body: `
              <div class="fact-grid">
                <div class="fact"><span class="ctx-label">repo</span><code>${escapeHtml(sharedData.repository)}</code></div>
                <div class="fact"><span class="ctx-label">branch</span><code>${escapeHtml(sharedData.branch)}</code></div>
                <div class="fact"><span class="ctx-label">base</span><code>${escapeHtml(sharedData.baseCommit)}</code></div>
                <div class="fact"><span class="ctx-label">commit</span><code>${escapeHtml(sharedData.commit)}</code></div>
                <div class="fact"><span class="ctx-label">attempt</span><span>1</span></div>
              </div>`,
          })}
          ${apiNote("partial", "GET /v1/submissions/{id} 与 /diff 已有；提交详情页仍需补齐。")}
          ${apiNote("planned", "验证 job 明细、attempt、lease 与日志需要新增只读查询。")}
        </div>
        <div class="col col--fill">
          ${card({
            title: "验证检查",
            sub: "8 项",
            pad: false,
            body: checks.map(checkRow).join(""),
          })}
          <p class="field-hint">仅平台错误提供“平台重试”；代码检查失败不提供重试。</p>
        </div>
      </div>
    </div>`;
  return appShell({ module: "提交验证", active: "审核", body });
}

// ------------------------------------------------------ 09 review-workspace

function reviewWorkspace() {
  const dims = [
    { name: "正确性", max: 30, score: 28 },
    { name: "测试充分性", max: 20, score: 15 },
    { name: "安全性", max: 15, score: 14 },
    { name: "可维护性", max: 15, score: 13 },
    { name: "变更范围", max: 10, score: 9 },
    { name: "文档", max: 10, score: 10 },
  ];
  const diff = diffViewer({
    file: `${sharedData.repository}/settlement/retry.go`,
    lines: [
      { old: "41", new: "41", text: "func Retry(ctx context.Context, req Request) error {" },
      { old: "42", text: "  return process(ctx, req)", kind: "del" },
      { new: "42", text: "  key := idempotencyKey(req)", kind: "add" },
      { new: "43", text: "  if seen(key) { return ErrDuplicate }", kind: "add" },
      { new: "44", text: "  return process(ctx, req)", kind: "add" },
      { old: "43", new: "45", text: "}" },
    ],
  });

  const left = `
    <div class="stack-sm">
      <div class="card-title">文件</div>
      <div class="file-row" data-selected="true"><span>retry.go</span><span class="faint text-xs">+4 −1</span></div>
      <div class="file-row"><span>retry_test.go</span><span class="faint text-xs">+22</span></div>
      <div class="file-row"><span>migrations/0007.sql</span><span class="faint text-xs">+8</span></div>
      <div class="review-progress faint text-xs">已审阅 1 / 3 文件</div>
    </div>`;

  const center = `
    <div class="stack">
      ${card({ title: "证据摘要", body: `<p class="muted text-sm">新增幂等键与去重判定，配套迁移与测试。隐藏测试 18/20，触发硬门槛告警。</p>` })}
      ${diff}
      ${card({ title: "行内评论", body: `<div class="line-comment"><strong>Lin（审核人）</strong><p class="text-sm muted">第 43 行：seen(key) 是否需要考虑并发写入下的竞态？</p></div>` })}
    </div>`;

  const right = `
    <div class="stack">
      ${card({ title: "自动检查", pad: false, body: [
        checkRow({ name: "公共测试", state: "pass", value: "42/42" }),
        checkRow({ name: "隐藏测试", state: "warn", value: "18/20" }),
        checkRow({ name: "安全扫描", state: "pass" }),
      ].join("") })}
      ${card({ title: "Rubric 评分", body: rubricForm({ dimensions: dims, total: 89 }) })}
      <div class="hard-gate" role="alert">硬门槛：隐藏测试未达标（18/20），“通过并评分”不可用。</div>
      <div class="row">
        ${button("通过并评分", { variant: "primary", disabled: true })}
        ${button("退回修改", { variant: "danger" })}
      </div>
      ${apiNote("available", "GET /v1/reviews/{id} · /rubrics/active 支撑审核详情与 Rubric。")}
      ${apiNote("auth-fix", "评论/结论当前 bearer-only；人类 Web 审核应支持 session 身份。")}
    </div>`;

  const body = `
    <div class="stack">
      ${pageHeader({ title: "审核工作台", sub: `${sharedData.taskId} · Diff 为审核中心` })}
      <div class="split-3">
        <div class="col scroll">${left}</div>
        <div class="col scroll">${center}</div>
        <div class="col scroll">${right}</div>
      </div>
    </div>`;
  return appShell({ module: "审核工作台", active: "审核", body });
}

// ------------------------------------------------------------- 10 outcome

function outcome() {
  const revision = `
    <div class="stack-sm">
      <div class="row-between"><strong>返工请求（并列状态）</strong>${statusChip("待返工", "warning")}</div>
      <ul class="perm-list">
        <li>2 条未解决审核评论</li>
        <li>Agent 正在准备下一版修订</li>
        <li>尚未提交新的 revision</li>
      </ul>
    </div>`;

  const body = `
    <div class="stack">
      ${pageHeader({ title: "结果闭环", sub: `${sharedData.taskId} · 已接受路径为主状态` })}
      ${card({
        title: "已接受",
        head: statusChip("已完成", "success"),
        body: `
          <div class="stack">
            ${metricGrid([
              { label: "审核评分", value: sharedData.reviewScore, positive: true },
              { label: "声望", value: sharedData.reputationDelta, positive: true },
              { label: "Issue 回写", value: "成功", positive: true },
              { label: "经验候选", value: "已创建" },
            ])}
            <ul class="perm-list">
              <li>Task 已完成，Execution 已接受</li>
              <li>GitHub Issue 回写成功（#412 已关闭并评论）</li>
              <li>经验候选已创建，等待评测</li>
            </ul>
          </div>`,
      })}
      <div class="split-2">
        <div class="col">${card({ title: "返工分支", body: revision })}</div>
        <div class="col">
          ${apiNote("partial", "GET /v1/reputation 与 Agent 经验/评测接口可查声望与经验。")}
          ${apiNote("planned", "outcome 聚合、Issue 回写状态与审计时间线待补。")}
        </div>
      </div>
    </div>`;
  return appShell({ module: "结果闭环", active: "结果", body });
}

export const flowScreens = Object.freeze({
  login,
  onboarding,
  "git-integration": gitIntegration,
  "repository-sync-rule": repositorySyncRule,
  "sync-result": syncResult,
  "task-center": taskCenter,
  "execution-detail": executionDetail,
  "submission-validation": submissionValidation,
  "review-workspace": reviewWorkspace,
  outcome,
});
