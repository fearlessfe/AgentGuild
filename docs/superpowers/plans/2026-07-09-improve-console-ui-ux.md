---
change: improve-console-ui-ux
design-doc: docs/superpowers/specs/2026-07-09-improve-console-ui-ux-design.md
base-ref: 116b13b4bea43bba1b3ee9172b6f782d860f9924
---

# Improve Console UI/UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the AgentGuild console's review, task, and Agent workspaces readable and touch-friendly across desktop and 375px mobile viewports.

**Architecture:** Preserve existing React Router, TanStack Query, CSS token, and feature-folder structure. Apply targeted component/style changes around review diffs, task/Agent list rendering, shell icons, mobile hit targets, and feedback states.

**Tech Stack:** React 19, TypeScript, Vite, TanStack Query, React Router, Playwright, Vitest, CSS modules via global project styles, `lucide-react` for SVG icons.

## Global Constraints

- No backend API, database, transport, or domain state-machine changes.
- Desktop workbench density must remain intact.
- Mobile review defaults to Unified mode while preserving mode controls.
- Mobile task and Agent lists use compact cards as the primary responsive presentation.
- Primary mobile interactive controls must provide a practical 44px by 44px hit area.
- Diff content must not wrap into character-by-character columns.
- Key mobile routes must not create page-level horizontal overflow.

---

### Task 1: Add UI/UX Regression Checks

**Files:**
- Create: `frontend/e2e/console-ui-ux.spec.ts`
- Read: `frontend/e2e/task-observer.visual.spec.ts`
- Read: `frontend/playwright.config.ts`

**Interfaces:**
- Consumes: existing demo mode routing and fixtures.
- Produces: helper functions `expectNoPageOverflow`, `expectReadableDiff`, and route checks used by later tasks.

- [x] **Step 1: Create the Playwright spec with failing checks**

Create `frontend/e2e/console-ui-ux.spec.ts`:

```ts
import { expect, test, type Page } from "@playwright/test";

async function expectNoPageOverflow(page: Page) {
  const metrics = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
  }));
  expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
  expect(metrics.bodyScrollWidth).toBeLessThanOrEqual(metrics.clientWidth);
}

async function expectReadableDiff(page: Page) {
  const measurements = await page.locator(".diff-table .line-content pre").evaluateAll((nodes) =>
    nodes
      .map((node) => {
        const rect = node.getBoundingClientRect();
        return {
          text: node.textContent ?? "",
          width: rect.width,
          height: rect.height,
        };
      })
      .filter((item) => item.text.trim().length > 0),
  );
  expect(measurements.length).toBeGreaterThan(0);
  for (const item of measurements.slice(0, 6)) {
    expect(item.width).toBeGreaterThan(80);
    expect(item.height).toBeLessThan(80);
  }
}

test.describe("console UI/UX baseline", () => {
  test("review diff remains readable on desktop", async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/reviews/review-1");
    await expect(page.getByRole("heading", { name: "审核工作台" })).toBeVisible();
    await expectReadableDiff(page);
    await expectNoPageOverflow(page);
  });

  test("review diff remains readable on mobile", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/reviews/review-1");
    await expect(page.getByRole("heading", { name: "审核工作台" })).toBeVisible();
    await expectReadableDiff(page);
    await expectNoPageOverflow(page);
  });

  test("mobile task list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/tasks");
    await expect(page.getByText("修复批量退款时的余额竞争条件")).toBeVisible();
    await expect(page.getByText("billing-service")).toBeVisible();
    await expect(page.getByText("待领取").first()).toBeVisible();
    await expectNoPageOverflow(page);
  });

  test("mobile agent list exposes core information", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto("/agents");
    await expect(page.getByText("Atlas v12")).toBeVisible();
    await expect(page.getByText("atlas@example.com")).toBeVisible();
    await expect(page.getByText("Active").first()).toBeVisible();
    await expectNoPageOverflow(page);
  });
});
```

- [x] **Step 2: Run the targeted spec to capture current failures**

Run:

```bash
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts
```

Expected: at least the diff readability check fails before implementation because the current split diff wraps code into narrow columns.

- [x] **Step 3: Commit the failing baseline**

```bash
git add frontend/e2e/console-ui-ux.spec.ts
git commit -m "test: capture console ui ux regressions"
```

### Task 2: Fix Review Diff Readability

**Files:**
- Modify: `frontend/src/features/reviews/DiffViewer.tsx`
- Modify: `frontend/src/styles/review.css`
- Test: `frontend/e2e/console-ui-ux.spec.ts`

**Interfaces:**
- Consumes: `DiffMode`, existing diff table markup, line-comment callbacks.
- Produces: mobile default Unified mode and readable diff region layout.

- [x] **Step 1: Add viewport-aware default mode**

In `frontend/src/features/reviews/DiffViewer.tsx`, add a small hook near the top of the file:

```tsx
function initialDiffMode() {
  if (typeof window !== "undefined" && window.matchMedia("(max-width: 700px)").matches) {
    return "unified" as const;
  }
  return "split" as const;
}
```

Change:

```tsx
const [mode, setMode] = useState<DiffMode>("split");
```

to:

```tsx
const [mode, setMode] = useState<DiffMode>(initialDiffMode);
```

- [x] **Step 2: Add a scroll container around diff tables**

Wrap both table branches in a container:

```tsx
<div className="diff-scroll">
  {mode === "unified" ? (
    <table className="diff-table unified">
      ...
    </table>
  ) : (
    <table className="diff-table split">
      ...
    </table>
  )}
</div>
```

- [x] **Step 3: Update diff CSS**

In `frontend/src/styles/review.css`, add:

```css
.diff-scroll {
  overflow-x: auto;
  overflow-y: visible;
  max-width: 100%;
}

.diff-table {
  min-width: 720px;
  table-layout: fixed;
}

.diff-table.unified {
  min-width: 560px;
}

.diff-table .line-content pre {
  margin: 0;
  padding: 0 12px;
  white-space: pre;
  overflow: visible;
  word-break: normal;
}

@media (max-width: 700px) {
  .diff-table.split {
    min-width: 720px;
  }

  .diff-table.unified {
    min-width: 520px;
  }
}
```

Remove or replace the existing `.diff-table .line-content pre` rule that uses `white-space: pre-wrap` and `word-break: break-word`.

- [x] **Step 4: Verify diff behavior**

Run:

```bash
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts --grep "review diff"
```

Expected: desktop and mobile diff readability checks pass.

- [x] **Step 5: Commit review diff readability change**

```bash
git add frontend/src/features/reviews/DiffViewer.tsx frontend/src/styles/review.css frontend/e2e/console-ui-ux.spec.ts
git commit -m "fix: keep review diffs readable"
```

### Task 3: Add Mobile Task And Agent Cards

**Files:**
- Modify: `frontend/src/features/tasks/TaskList.tsx`
- Modify: `frontend/src/features/agents/AgentList.tsx`
- Modify: `frontend/src/styles/components.css`
- Modify: `frontend/src/styles/legacy.css`
- Modify: `frontend/src/styles/responsive.css`
- Test: `frontend/e2e/console-ui-ux.spec.ts`

**Interfaces:**
- Consumes: existing task and agent query/filter state.
- Produces: `.task-mobile-list`, `.task-mobile-card`, `.agent-mobile-list`, and `.agent-mobile-card`.

- [x] **Step 1: Render mobile task cards beside the desktop table**

In `TaskList.tsx`, after the desktop table section, render:

```tsx
<div className="task-mobile-list" aria-label="移动任务列表">
  {groups.flatMap((group) =>
    group.items.map((task) => (
      <Link className="task-mobile-card" key={task.id} to={`/tasks/${task.id}`} aria-label={`查看 ${task.id}`}>
        <div className="row-between">
          <code>{task.id}</code>
          <StatusChip tone={statusTone[task.status]}>{statusLabel[task.status]}</StatusChip>
        </div>
        <strong>{task.title}</strong>
        <span className="muted text-sm">{task.publisher_agent_version_id}</span>
        <span className="faint text-xs">{formatDeadline(task.deadline)}</span>
      </Link>
    )),
  )}
</div>
```

- [x] **Step 2: Render mobile Agent cards beside the desktop table**

In `AgentList.tsx`, after `.agent-table`, render:

```tsx
<div className="agent-mobile-list" aria-label="移动 Agent 列表">
  {agents.map((agent) => (
    <Link className="agent-mobile-card" key={agent.id} to={`/agents/${agent.id}`} aria-label={`查看 ${agent.name}`}>
      <div className="row-between">
        <strong>{agent.name}</strong>
        <span className="agent-status-cell">
          <span className={`status-dot ${agent.status}`} aria-hidden="true" />
          <span className={`agent-status-text ${agent.status}`}>{formatStatus(agent.status)}</span>
        </span>
      </div>
      <span className="muted text-sm">{agent.owner_email}</span>
      <span className="faint text-xs">{agent.team ?? "未分配"}</span>
    </Link>
  ))}
</div>
```

- [x] **Step 3: Add responsive styles**

In `frontend/src/styles/components.css`:

```css
.task-mobile-list {
  display: none;
}

.task-mobile-card {
  display: grid;
  gap: var(--sp-2);
  padding: var(--sp-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--text);
  text-decoration: none;
}
```

In `frontend/src/styles/legacy.css`:

```css
.agent-mobile-list {
  display: none;
}

.agent-mobile-card {
  display: grid;
  gap: var(--sp-2);
  padding: var(--sp-4);
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: var(--surface);
  color: var(--text);
  text-decoration: none;
}
```

In `frontend/src/styles/responsive.css` inside `@media (max-width: 700px)`:

```css
.dense-table,
.agent-table {
  display: none;
}

.task-mobile-list,
.agent-mobile-list {
  display: grid;
  gap: var(--sp-3);
}
```

- [x] **Step 4: Verify mobile list checks**

Run:

```bash
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts --grep "mobile .* list"
```

Expected: task and Agent mobile list checks pass.

- [x] **Step 5: Commit mobile workbench cards**

```bash
git add frontend/src/features/tasks/TaskList.tsx frontend/src/features/agents/AgentList.tsx frontend/src/styles/components.css frontend/src/styles/legacy.css frontend/src/styles/responsive.css frontend/e2e/console-ui-ux.spec.ts
git commit -m "feat: add mobile workbench cards"
```

### Task 4: Replace Structural Icons And Improve Mobile Hit Targets

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/src/app/Rail.tsx`
- Modify: `frontend/src/app/Topbar.tsx`
- Modify: `frontend/src/ui/EmptyState.tsx`
- Modify: `frontend/src/ui/ProviderCard.tsx`
- Modify: `frontend/src/features/tasks/TaskDetail.tsx`
- Modify: `frontend/src/features/git/GitIntegrationScreen.tsx`
- Modify: `frontend/src/styles/shell.css`
- Modify: `frontend/src/styles/components.css`
- Modify: `frontend/src/styles/responsive.css`

**Interfaces:**
- Consumes: existing shell/navigation props and shared UI primitive APIs.
- Produces: consistent Lucide icon usage and mobile hit-target CSS.

- [ ] **Step 1: Add the icon package**

Run:

```bash
cd frontend && npm install lucide-react
```

Expected: `frontend/package.json` and `frontend/package-lock.json` include `lucide-react`.

- [ ] **Step 2: Replace Rail icons**

In `Rail.tsx`, import Lucide icons and change item shape:

```tsx
import { Bot, CircleDot, GitBranch, GitPullRequest, LayoutDashboard, Settings, SquareKanban } from "lucide-react";
import type { LucideIcon } from "lucide-react";

type RailItem = { to: string; icon: LucideIcon; label: string };

const RAIL_ITEMS: readonly RailItem[] = [
  { to: "/onboarding", icon: LayoutDashboard, label: "总览" },
  { to: "/sync", icon: GitBranch, label: "同步" },
  { to: "/tasks", icon: SquareKanban, label: "任务中心" },
  { to: "/reviews", icon: GitPullRequest, label: "审核" },
  { to: "/outcome", icon: CircleDot, label: "结果" },
];
```

Render icons with:

```tsx
const Icon = item.icon;
<span className="rail-icon" aria-hidden="true">
  <Icon size={17} strokeWidth={1.8} />
</span>
```

Use `Bot` for Agents and `Settings` for the static settings item.

- [ ] **Step 3: Replace Topbar icons**

In `Topbar.tsx`, import:

```tsx
import { CircleHelp, Moon, Search, Sun } from "lucide-react";
```

Replace `⌕`, `◐`, `◑`, and `?` with `Search`, `Moon`, `Sun`, and `CircleHelp`, keeping existing aria labels.

- [ ] **Step 4: Replace shared surface icons**

Update shared UI surfaces that currently accept Unicode icon strings:

```tsx
// EmptyState default
import { CircleDot } from "lucide-react";

// ProviderCard accepts ReactNode logo instead of only text
type ProviderCardProps = {
  logo: React.ReactNode;
  name: string;
  note?: string;
  permissions: string[];
  disabled?: boolean;
};
```

Update `GitIntegrationScreen.tsx` provider logos to pass Lucide nodes instead of `"◐"` and `"◑"`. Update `TaskDetail.Empty` to render the shared `EmptyState` with a Lucide `SquareKanban` icon node instead of the inline `◱` character.

- [ ] **Step 5: Add mobile hit-target CSS**

In `responsive.css` inside `@media (max-width: 700px)`:

```css
.rail-item,
.rail-toggle,
.icon-btn,
.tab,
.btn,
.primary-action,
.secondary-action,
.danger-action {
  min-width: 44px;
  min-height: 44px;
  touch-action: manipulation;
}

.tabs {
  gap: var(--sp-2);
  overflow-x: auto;
}

.filter select,
.filter input,
.agent-toolbar select,
.field input,
.field select,
.field textarea {
  min-height: 44px;
}
```

- [ ] **Step 6: Verify shell still renders**

Run:

```bash
cd frontend && npm test -- --run
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts
```

Expected: unit tests and UI/UX e2e checks pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/package.json frontend/package-lock.json frontend/src/app/Rail.tsx frontend/src/app/Topbar.tsx frontend/src/ui/EmptyState.tsx frontend/src/ui/ProviderCard.tsx frontend/src/features/tasks/TaskDetail.tsx frontend/src/features/git/GitIntegrationScreen.tsx frontend/src/styles/shell.css frontend/src/styles/components.css frontend/src/styles/responsive.css
git commit -m "feat: standardize console icons and mobile targets"
```

### Task 5: Improve Feedback States

**Files:**
- Modify: `frontend/src/features/reviews/ReviewPage.tsx`
- Modify: `frontend/src/features/reviews/DiffViewer.tsx`
- Modify: `frontend/src/features/agents/AgentRegister.tsx`
- Modify: `frontend/src/styles/components.css`
- Modify: `frontend/src/styles/review.css`
- Test: existing feature tests and `frontend/e2e/console-ui-ux.spec.ts`

**Interfaces:**
- Consumes: existing TanStack Query `isPending` and mutation state.
- Produces: clear pending/disabled/recovery UI for review and Agent mutations.

- [ ] **Step 1: Add a shared pending label pattern**

In affected buttons, change button copy during pending states:

```tsx
{decisionMutation.isPending ? "提交中…" : "通过"}
```

Apply equivalent copy to revision/reject decision buttons, comment submission, and Agent registration submit.

- [ ] **Step 2: Ensure duplicate submission is blocked**

For review decision buttons, keep or add:

```tsx
disabled={decisionMutation.isPending || rubricQuery.isPending}
```

For comment submission in `DiffViewer.tsx`, disable submit while the promise is pending by adding local state:

```tsx
const [submittingComment, setSubmittingComment] = useState(false);
```

Wrap `submitComment` with `setSubmittingComment(true)` and reset in `finally`.

- [ ] **Step 3: Add visible recovery paths**

For mutation errors, ensure each affected form renders a local `role="alert"` message with the error and leaves the user's input intact. Use existing `.auth-error` or add a shared `.form-error` style:

```css
.form-error {
  padding: 8px 12px;
  border: 1px solid color-mix(in srgb, var(--danger) 45%, transparent);
  border-radius: var(--r-sm);
  background: var(--danger-quiet);
  color: var(--danger);
  font-size: 12px;
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
cd frontend && npm test -- --run
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts
```

Expected: tests pass and pending states do not allow duplicate submission.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/features/reviews/ReviewPage.tsx frontend/src/features/reviews/DiffViewer.tsx frontend/src/features/agents/AgentRegister.tsx frontend/src/styles/components.css frontend/src/styles/review.css
git commit -m "fix: clarify console pending and error states"
```

### Task 6: Final Verification And Documentation Sync

**Files:**
- Modify: `openspec/changes/improve-console-ui-ux/tasks.md`
- Read: `docs/superpowers/specs/2026-07-09-improve-console-ui-ux-design.md`
- Read: `openspec/changes/improve-console-ui-ux/specs/console-ui-ux/spec.md`

**Interfaces:**
- Consumes: all earlier task outputs.
- Produces: verified implementation with OpenSpec task checkboxes updated.

- [ ] **Step 1: Run full frontend verification**

Run:

```bash
cd frontend && npm run build
cd frontend && npm test -- --run
cd frontend && npx playwright test e2e/console-ui-ux.spec.ts
```

Expected: all commands pass.

- [ ] **Step 2: Inspect screenshots**

Run the dev server in demo mode and inspect `/tasks`, `/agents`, and `/reviews/review-1` at 1440px and 375px widths:

```bash
cd frontend && VITE_DEMO_MODE=true npm run dev -- --host 127.0.0.1
```

Expected: no character-level diff wrapping, mobile cards expose core information, and no page-level horizontal overflow.

- [ ] **Step 3: Mark OpenSpec tasks complete**

After verification, mark all relevant checkboxes in `openspec/changes/improve-console-ui-ux/tasks.md` as complete.

- [ ] **Step 4: Commit verification state**

```bash
git add openspec/changes/improve-console-ui-ux/tasks.md
git commit -m "chore: complete console ui ux tasks"
```
