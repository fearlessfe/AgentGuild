import {
  SCREEN_IDS,
  apiCapabilities,
  apiStatus,
  screenRegistry,
} from "./screens.js";

const THEMES = new Set(["dark", "light"]);

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function apiCoverageBoard() {
  const legend = Object.entries(apiStatus)
    .map(
      ([key, item]) => `
        <li>
          <span class="api-chip api-chip--${key}">${escapeHtml(item.label)}</span>
          <span>${escapeHtml(item.description)}</span>
        </li>`,
    )
    .join("");

  const rows = apiCapabilities
    .map((row) => {
      const status = apiStatus[row.status];
      return `
        <tr>
          <th scope="row">${escapeHtml(row.capability)}</th>
          <td><code>${escapeHtml(row.endpoint)}</code></td>
          <td><span class="api-chip api-chip--${row.status}">${escapeHtml(status.label)}</span></td>
          <td>${escapeHtml(row.note)}</td>
        </tr>`;
    })
    .join("");

  return `
    <article class="screen annotation-board" aria-labelledby="api-coverage-title">
      <header class="board-header">
        <div>
          <p class="eyebrow">DELIVERY ANNOTATION · 00</p>
          <h1 id="api-coverage-title">API 能力覆盖</h1>
          <p class="lede">将全流程界面与当前 REST 能力逐项对齐。标注仅供设计交付与实施评审，不属于用户界面。</p>
        </div>
        <p class="board-count"><strong>${apiCapabilities.length}</strong><span>项能力</span></p>
      </header>
      <ul class="api-legend" aria-label="API 状态图例">${legend}</ul>
      <div class="coverage-table-wrap">
        <table class="coverage-table">
          <caption class="visually-hidden">AgentGuild API 能力覆盖矩阵，共 15 行</caption>
          <colgroup>
            <col class="col-capability" />
            <col class="col-endpoint" />
            <col class="col-status" />
            <col class="col-note" />
          </colgroup>
          <thead>
            <tr>
              <th scope="col">页面能力</th>
              <th scope="col">当前接口</th>
              <th scope="col">交付标注</th>
              <th scope="col">设计约束</th>
            </tr>
          </thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
      <footer class="board-footer">
        <span>Source · approved full-product-flow visual spec</span>
        <span>Canonical canvas · 1440 × 1024</span>
      </footer>
    </article>`;
}

function placeholderScreen(screenId) {
  const screen = screenRegistry[screenId];
  return `
    <article class="screen placeholder-screen" aria-labelledby="${escapeHtml(screenId)}-title">
      <p class="eyebrow">AGENTGUILD · FULL FLOW</p>
      <h1 id="${escapeHtml(screenId)}-title">${escapeHtml(screen.title)}</h1>
      <p>此画布已登记，将在后续任务中补充高保真内容。</p>
      <p class="route-label"><code>?screen=${escapeHtml(screenId)}&amp;theme=&lt;dark|light&gt;</code></p>
    </article>`;
}

function galleryScreen(theme) {
  const cards = SCREEN_IDS.filter((id) => id !== "gallery")
    .map(
      (id) => `
        <a class="gallery-card" href="?screen=${escapeHtml(id)}&amp;theme=${theme}">
          <span>${escapeHtml(screenRegistry[id].kind)}</span>
          <strong>${escapeHtml(screenRegistry[id].title)}</strong>
          <code>${escapeHtml(id)}</code>
        </a>`,
    )
    .join("");

  return `
    <article class="screen gallery-screen" aria-labelledby="gallery-title">
      <header class="board-header">
        <div>
          <p class="eyebrow">AGENTGUILD · FULL FLOW</p>
          <h1 id="gallery-title">设计画廊</h1>
          <p class="lede">选择一块画布。深浅主题只替换语义 token，不改变几何结构。</p>
        </div>
      </header>
      <nav class="gallery-grid" aria-label="设计画布">${cards}</nav>
    </article>`;
}

function unknownScreen(screenId, theme) {
  const links = SCREEN_IDS.map(
    (id) =>
      `<a href="?screen=${escapeHtml(id)}&amp;theme=${theme}"><code>${escapeHtml(id)}</code></a>`,
  ).join("");

  return `
    <section class="screen route-error" role="alert" aria-labelledby="route-error-title">
      <p class="eyebrow">ROUTE ERROR</p>
      <h1 id="route-error-title">未找到画布 “${escapeHtml(screenId)}”</h1>
      <p>请使用以下有效 screen ID：</p>
      <nav aria-label="有效画布 ID">${links}</nav>
    </section>`;
}

export function renderScreen(screenId, theme = "dark") {
  const safeTheme = THEMES.has(theme) ? theme : "dark";
  document.documentElement.dataset.theme = safeTheme;

  if (!SCREEN_IDS.includes(screenId)) {
    return unknownScreen(screenId, safeTheme);
  }
  if (screenId === "api-coverage") {
    return apiCoverageBoard();
  }
  if (screenId === "gallery") {
    return galleryScreen(safeTheme);
  }
  return placeholderScreen(screenId);
}

export function readRoute(search = window.location.search) {
  const params = new URLSearchParams(search);
  const screenId = params.get("screen") || "gallery";
  const requestedTheme = params.get("theme") || "dark";
  return {
    screenId,
    theme: THEMES.has(requestedTheme) ? requestedTheme : "dark",
  };
}

function mount() {
  const app = document.querySelector("#app");
  if (!app) {
    throw new Error("Missing #app mount point");
  }
  const route = readRoute();
  app.innerHTML = renderScreen(route.screenId, route.theme);
  document.title = `${screenRegistry[route.screenId]?.title || "路由错误"} · AgentGuild`;
}

window.renderScreen = renderScreen;
mount();
