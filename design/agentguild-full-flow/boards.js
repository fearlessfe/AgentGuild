// Documentation boards: end-to-end journey, component/state matrix, and
// responsive rules. These are design-review artifacts, not user UI.

import {
  button,
  checkRow,
  escapeHtml,
  emptyState,
  statusChip,
} from "./components.js";

// ------------------------------------------------------------- journey

const LANES = [
  {
    name: "人类治理",
    tone: "info",
    nodes: ["登录", "Git 接入", "仓库与规则", "同步预览", "人工审核", "通过 / 返工"],
  },
  {
    name: "平台编排",
    tone: "action",
    nodes: ["Issue 同步", "任务中心", "提交验证", "Issue / 声望 / 经验闭环"],
  },
  {
    name: "Agent 执行",
    tone: "neutral",
    nodes: ["领取任务", "执行与提交"],
  },
];

function journey() {
  const lanes = LANES.map(
    (lane) => `
      <div class="lane">
        <div class="lane-head">${statusChip(lane.name, lane.tone)}</div>
        <div class="lane-track">
          ${lane.nodes.map((n) => `<span class="flow-node">${escapeHtml(n)}</span>`).join('<span class="flow-arrow" aria-hidden="true">→</span>')}
        </div>
      </div>`,
  ).join("");

  return `
    <article class="screen annotation-board journey-board" aria-labelledby="journey-title">
      <header class="board-header">
        <div>
          <p class="eyebrow">AGENTGUILD · FULL FLOW</p>
          <h1 id="journey-title">全流程总览</h1>
          <p class="lede">人类配置与审核，平台编排同步与验证，Agent 领取并执行——Agent 专属动作不出现在人类交互路径上。</p>
        </div>
      </header>
      <div class="journey">${lanes}</div>
      <div class="journey-flow">
        <span class="ctx-label">端到端路径</span>
        <p class="mono text-xs faint">登录 → Git 接入 → 仓库与规则 → 同步预览 → 任务中心 → Agent 领取与执行 → 提交验证 → 人工审核 → 通过/返工 → Issue/声望/经验闭环</p>
      </div>
      <footer class="board-footer">
        <span>Responsibility lanes · 人类治理 / 平台编排 / Agent 执行</span>
        <span>Canonical canvas · 1440 × 1024</span>
      </footer>
    </article>`;
}

// ------------------------------------------------------- components-states

const STATES = [
  { name: "default", demo: button("默认操作", {}) },
  { name: "hover", demo: button("悬停操作", { variant: "primary" }) },
  { name: "focus", demo: '<span class="input input--focus" style="width:120px">聚焦</span>' },
  { name: "selected", demo: statusChip("已选中", "action") },
  { name: "disabled", demo: button("不可用", { disabled: true }) },
  { name: "loading", demo: '<span class="mono faint">载入中 …</span>' },
  { name: "empty", demo: emptyState({ title: "暂无数据", text: "" }) },
  { name: "success", demo: statusChip("通过", "success") },
  { name: "warning", demo: statusChip("告警", "warning") },
  { name: "danger", demo: statusChip("失败", "danger") },
  { name: "permission expired", demo: statusChip("权限过期", "danger") },
  { name: "sync conflict", demo: statusChip("同步冲突", "warning") },
  {
    name: "validation hard gate",
    demo: checkRow({ name: "隐藏测试", state: "warn", value: "18/20" }),
  },
];

function componentsStates() {
  const cells = STATES.map(
    (s) => `
      <div class="state-cell">
        <span class="state-name">${escapeHtml(s.name)}</span>
        <div class="state-demo">${s.demo}</div>
      </div>`,
  ).join("");

  return `
    <article class="screen annotation-board" aria-labelledby="states-title">
      <header class="board-header">
        <div>
          <p class="eyebrow">AGENTGUILD · FULL FLOW</p>
          <h1 id="states-title">组件与状态</h1>
          <p class="lede">深浅主题只替换语义 token，不改变几何。焦点态使用可见轮廓，状态均带文字或图标。</p>
        </div>
        <p class="board-count"><strong>${STATES.length}</strong><span>状态</span></p>
      </header>
      <div class="state-grid">${cells}</div>
      <div></div>
      <footer class="board-footer">
        <span>键盘可达 · 焦点轮廓 · 语义色不单独承载信息</span>
        <span>Dark / Light 共享几何</span>
      </footer>
    </article>`;
}

// -------------------------------------------------------- responsive-rules

const BREAKPOINTS = [
  { w: "1440", title: "三栏工作台", note: "文件树 / Diff / 检查·Rubric·结论 全部展开。", cols: [24, 52, 24] },
  { w: "1280", title: "紧凑三栏", note: "列宽收窄，Diff 仍为视觉中心。", cols: [22, 56, 22] },
  { w: "1024", title: "两栏 + 详情抽屉", note: "右栏收入抽屉，主区保留列表与 Diff。", cols: [34, 66] },
  { w: "768", title: "单主列", note: "详情走路由；底部审核抽屉承载结论操作。", cols: [100] },
];

function responsiveRules() {
  const frames = BREAKPOINTS.map(
    (bp) => `
      <div class="bp">
        <div class="bp-head"><strong>${bp.w}</strong><span class="faint text-xs">${escapeHtml(bp.title)}</span></div>
        <div class="bp-frame">
          ${bp.cols.map((c) => `<span class="bp-col" style="flex:${c}"></span>`).join("")}
        </div>
        <p class="bp-note">${escapeHtml(bp.note)}</p>
      </div>`,
  ).join("");

  return `
    <article class="screen annotation-board" aria-labelledby="responsive-title">
      <header class="board-header">
        <div>
          <p class="eyebrow">AGENTGUILD · FULL FLOW</p>
          <h1 id="responsive-title">响应式规则</h1>
          <p class="lede">1440 为基准宽度，向下适配 1280 / 1024 / 768。低于 720px 为只读回退范围，不作独立高保真目标。</p>
        </div>
      </header>
      <div class="bp-grid">${frames}</div>
      <div></div>
      <footer class="board-footer">
        <span>共享组件层级 · 断点仅改变布局密度</span>
        <span>&lt; 720px · 只读回退</span>
      </footer>
    </article>`;
}

export const boardScreens = Object.freeze({
  journey,
  "components-states": componentsStates,
  "responsive-rules": responsiveRules,
});
