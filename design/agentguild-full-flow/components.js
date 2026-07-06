// Reusable HTML component renderers for the AgentGuild full-flow gallery.
// Every renderer returns an HTML string and reads only from semantic CSS
// tokens — no screen-specific color literals live here.

export function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

const RAIL_ITEMS = Object.freeze([
  { icon: "◆", label: "总览" },
  { icon: "⤳", label: "同步" },
  { icon: "◱", label: "任务中心" },
  { icon: "❖", label: "审核" },
  { icon: "◔", label: "结果" },
]);

// ------------------------------------------------------------ app shell

export function appShell({ module, context = [], body, active = "" }) {
  return `
    <div class="app">
      ${rail(active)}
      <div class="app-body">
        ${topbar(module)}
        ${context.length ? contextbar(context) : ""}
        <div class="page scroll">${body}</div>
      </div>
    </div>`;
}

function rail(active) {
  const items = RAIL_ITEMS.map(
    (item) => `
      <a class="rail-item" href="#" data-active="${item.label === active}" aria-label="${escapeHtml(item.label)}">
        <span aria-hidden="true">${item.icon}</span>
        <span class="rail-label">${escapeHtml(item.label)}</span>
      </a>`,
  ).join("");
  return `
    <nav class="rail" aria-label="主导航">
      <span class="rail-brand" aria-hidden="true">AG</span>
      ${items}
      <span class="rail-spacer"></span>
      <a class="rail-item" href="#" aria-label="设置"><span aria-hidden="true">⚙</span><span class="rail-label">设置</span></a>
    </nav>`;
}

function topbar(module) {
  return `
    <header class="topbar">
      <span class="topbar-brand">AgentGuild</span>
      ${module ? `<span class="topbar-module">${escapeHtml(module)}</span>` : ""}
      <span class="topbar-spacer"></span>
      <span class="topbar-search">
        <span aria-hidden="true">⌕</span>
        <span>搜索任务、仓库、审核…</span>
        <kbd>⌘K</kbd>
      </span>
      <span class="topbar-icons">
        <span class="icon-btn" aria-hidden="true">◑</span>
        <span class="icon-btn" aria-hidden="true">⁋</span>
        <span class="avatar">L</span>
      </span>
    </header>`;
}

function contextbar(context) {
  const items = context
    .map(
      (item, index) => `
        ${index ? '<span class="ctx-sep" aria-hidden="true"></span>' : ""}
        <span class="ctx">
          <span class="ctx-label">${escapeHtml(item.label)}</span>
          <span class="ctx-value">${item.mono ? `<code>${escapeHtml(item.value)}</code>` : escapeHtml(item.value)}</span>
        </span>`,
    )
    .join("");
  return `<div class="contextbar">${items}</div>`;
}

// ------------------------------------------------------------ page header

export function pageHeader({ title, sub, actions = "" }) {
  return `
    <div class="page-header">
      <div>
        <div class="page-title">${escapeHtml(title)}</div>
        ${sub ? `<div class="page-sub">${escapeHtml(sub)}</div>` : ""}
      </div>
      ${actions ? `<div class="page-actions">${actions}</div>` : ""}
    </div>`;
}

// -------------------------------------------------------------- chips etc

export function statusChip(text, tone = "neutral") {
  return `<span class="status-chip status-chip--${tone}">${escapeHtml(text)}</span>`;
}

export function apiNote(status, text) {
  const labels = {
    available: "API · available",
    partial: "API · partial",
    planned: "API · planned",
    "auth-fix": "API · auth fix",
  };
  return `
    <p class="api-note" data-annotation="true">
      <span class="api-chip api-chip--${status}">${escapeHtml(labels[status] || status)}</span>
      <span>${escapeHtml(text)}</span>
    </p>`;
}

export function button(text, { variant = "", icon = "", disabled = false, block = false, lg = false } = {}) {
  const classes = [
    "btn",
    variant ? `btn--${variant}` : "",
    block ? "btn--block" : "",
    lg ? "btn--lg" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return `<span class="${classes}"${disabled ? ' data-disabled="true"' : ""}>${icon ? `<span aria-hidden="true">${icon}</span>` : ""}${escapeHtml(text)}</span>`;
}

export function card({ title, sub, head = "", body, pad = true }) {
  const header =
    title || head
      ? `<div class="card-head"><div><div class="card-title">${escapeHtml(title || "")}</div>${sub ? `<div class="card-sub">${escapeHtml(sub)}</div>` : ""}</div>${head}</div>`
      : "";
  return `<section class="card">${header}<div${pad ? ' class="card-pad"' : ""}>${body}</div></section>`;
}

// --------------------------------------------------------------- steps

export function steps(items) {
  const rows = items
    .map(
      (item, index) => `
        <li class="step" data-state="${item.state}">
          <span class="step-num">${item.state === "done" ? "✓" : index + 1}</span>
          <div>
            <div class="step-title">${escapeHtml(item.title)}</div>
            ${item.desc ? `<div class="step-desc">${escapeHtml(item.desc)}</div>` : ""}
          </div>
        </li>`,
    )
    .join("");
  return `<ol class="steps">${rows}</ol>`;
}

// ---------------------------------------------------------- dense table

export function denseTable({ columns, rows, caption = "" }) {
  const head = columns.map((c) => `<th scope="col">${escapeHtml(c)}</th>`).join("");
  const body = rows
    .map((row) => {
      if (row.group) {
        return `<tr class="group-row"><td colspan="${columns.length}">${escapeHtml(row.group)}</td></tr>`;
      }
      const cells = row.cells
        .map((cell) => `<td>${cell.html ?? escapeHtml(cell.text ?? cell)}</td>`)
        .join("");
      return `<tr data-selected="${row.selected ? "true" : "false"}">${cells}</tr>`;
    })
    .join("");
  return `
    <table class="dense-table">
      ${caption ? `<caption class="visually-hidden">${escapeHtml(caption)}</caption>` : ""}
      <thead><tr>${head}</tr></thead>
      <tbody>${body}</tbody>
    </table>`;
}

// -------------------------------------------------------------- timeline

export function timeline(events) {
  const rows = events
    .map(
      (event) => `
        <li>
          <div class="tl-time">${escapeHtml(event.time)}</div>
          <div class="tl-title">${escapeHtml(event.title)}</div>
          ${event.note ? `<div class="tl-note">${escapeHtml(event.note)}</div>` : ""}
        </li>`,
    )
    .join("");
  return `<ul class="timeline">${rows}</ul>`;
}

// ------------------------------------------------------------ empty state

export function emptyState({ icon = "◔", title, text }) {
  return `
    <div class="empty-state">
      <span class="es-icon" aria-hidden="true">${icon}</span>
      <div class="es-title">${escapeHtml(title)}</div>
      ${text ? `<div>${escapeHtml(text)}</div>` : ""}
    </div>`;
}

// ----------------------------------------------------------- provider card

export function providerCard({ logo, name, note, permissions = [], disabled = false, footer = "" }) {
  const perms = permissions.length
    ? `<ul class="perm-list">${permissions.map((p) => `<li>${escapeHtml(p)}</li>`).join("")}</ul>`
    : "";
  return `
    <div class="provider-card" data-disabled="${disabled}">
      <div class="provider-head">
        <span class="provider-logo" aria-hidden="true">${logo}</span>
        <div>
          <div class="provider-name">${escapeHtml(name)}</div>
          ${note ? `<div class="card-sub">${escapeHtml(note)}</div>` : ""}
        </div>
      </div>
      ${perms}
      ${footer}
    </div>`;
}

// -------------------------------------------------------------- check row

export function checkRow({ name, meta, state, value = "" }) {
  const marks = { pass: "✓", fail: "✕", warn: "!", run: "…" };
  return `
    <div class="check-row">
      <span class="check-mark check-mark--${state}" aria-hidden="true">${marks[state]}</span>
      <div>
        <div class="check-name">${escapeHtml(name)}</div>
        ${meta ? `<div class="check-meta">${escapeHtml(meta)}</div>` : ""}
      </div>
      ${value ? `<span class="check-meta">${escapeHtml(value)}</span>` : ""}
    </div>`;
}

// ------------------------------------------------------------ diff viewer

export function diffViewer({ file, lines }) {
  const rendered = lines
    .map((line) => {
      const kind = line.kind ?? "ctx";
      const cls = kind === "add" ? "diff-line--add" : kind === "del" ? "diff-line--del" : "";
      return `
        <div class="diff-line ${cls}">
          <span class="gut">${line.old ?? ""}</span>
          <span class="gut">${line.new ?? ""}</span>
          <span class="code">${escapeHtml(line.text)}</span>
        </div>`;
    })
    .join("");
  return `
    <div class="diff">
      <div class="diff-file">${escapeHtml(file)}</div>
      ${rendered}
    </div>`;
}

// ------------------------------------------------------------- rubric form

export function rubricForm({ dimensions, total, max = 100 }) {
  const rows = dimensions
    .map(
      (dim) => `
        <div class="rubric-row">
          <div>
            <div class="rubric-dim">${escapeHtml(dim.name)}</div>
            <div class="rubric-max">满分 ${dim.max}</div>
          </div>
          <div class="rubric-max">/ ${dim.max}</div>
          <div class="rubric-score">${dim.score}</div>
        </div>`,
    )
    .join("");
  return `
    <div>
      ${rows}
      <div class="rubric-total">
        <span class="unit">总分</span>
        <span><strong>${total}</strong> <span class="unit">/ ${max}</span></span>
      </div>
    </div>`;
}

// ---------------------------------------------------------- outcome summary

export function metricGrid(metrics) {
  const cells = metrics
    .map(
      (m) => `
        <div class="metric">
          <span class="metric-label">${escapeHtml(m.label)}</span>
          <span class="metric-value${m.positive ? " metric-value--pos" : ""}">${escapeHtml(m.value)}</span>
        </div>`,
    )
    .join("");
  return `<div class="metric-grid">${cells}</div>`;
}

export function tabs(items) {
  return `<div class="tabs">${items
    .map((t) => `<span class="tab" data-active="${t.active ? "true" : "false"}">${escapeHtml(t.label)}</span>`)
    .join("")}</div>`;
}

export function filterBar(items) {
  return `<div class="filter-bar">${items
    .map((f) => `<span class="filter">${escapeHtml(f.label)}${f.value ? `：<strong>${escapeHtml(f.value)}</strong>` : ""}</span>`)
    .join("")}</div>`;
}
