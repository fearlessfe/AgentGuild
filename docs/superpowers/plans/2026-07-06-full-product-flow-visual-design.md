# AgentGuild Full Product Flow Visual Design Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce an editable Pencil source file, a complete end-to-end journey board, 20 desktop high-fidelity screens in dark and light themes, an API coverage board, and responsive/state guidance for AgentGuild.

**Architecture:** Build one token-driven component system in a single `.pen` source file, then compose ten canonical 1440px desktop screens and apply dark/light theme variables without changing layout. Keep API feasibility annotations on a separate design-notes layer and export user-facing screens without those annotations.

**Tech Stack:** Pencil `.pen` editor and MCP tools, PNG/PDF export, existing Chinese AgentGuild product copy, current REST router/OpenAPI contracts, `docs/assets` reference images.

## Global Constraints

- Preserve “Agent 操作，人类治理”: human UI must not expose task claim, execution heartbeat, Git credential issuance, or Agent submission actions.
- Use a generic “Git 提供商” shell; GitHub is the only enabled provider and GitLab is visibly unavailable.
- Task synchronization is rule-driven with preview, pause, retry, and conflict handling.
- Produce exactly ten canonical pages in both themes: 20 user-facing desktop screens.
- Use 1440px as the canonical desktop width and document behavior at 1280px, 1024px, and 768px.
- Dark and light screens must share geometry, component hierarchy, copy, and semantic status.
- Do not display real tokens, private keys, credentials, tenant identifiers, or personal data.
- Use real current response fields when available; mark planned fields only on the design-notes layer.
- API annotation labels are `API · available`, `API · partial`, `API · planned`, and `API · auth fix`.
- The design plan does not implement backend API gaps. Repository listing, Issue synchronization, onboarding persistence, validation detail queries, Issue write-back, and review session-auth fixes require separate software implementation plans.
- Preserve all unrelated working-tree changes.

## File Map

**Create:**

- `docs/assets/agentguild-full-flow.pen` — editable source containing foundations, components, journey, API matrix, responsive rules, and all screens.
- `docs/assets/full-flow/00-full-flow-journey.png` — full process and responsibility-lane overview.
- `docs/assets/full-flow/00-api-coverage.png` — current/partial/planned/auth-fix interface coverage board.
- `docs/assets/full-flow/01-login-dark.png`
- `docs/assets/full-flow/01-login-light.png`
- `docs/assets/full-flow/02-onboarding-dark.png`
- `docs/assets/full-flow/02-onboarding-light.png`
- `docs/assets/full-flow/03-git-integration-dark.png`
- `docs/assets/full-flow/03-git-integration-light.png`
- `docs/assets/full-flow/04-repository-sync-rule-dark.png`
- `docs/assets/full-flow/04-repository-sync-rule-light.png`
- `docs/assets/full-flow/05-sync-result-dark.png`
- `docs/assets/full-flow/05-sync-result-light.png`
- `docs/assets/full-flow/06-task-center-dark.png`
- `docs/assets/full-flow/06-task-center-light.png`
- `docs/assets/full-flow/07-execution-detail-dark.png`
- `docs/assets/full-flow/07-execution-detail-light.png`
- `docs/assets/full-flow/08-submission-validation-dark.png`
- `docs/assets/full-flow/08-submission-validation-light.png`
- `docs/assets/full-flow/09-review-workspace-dark.png`
- `docs/assets/full-flow/09-review-workspace-light.png`
- `docs/assets/full-flow/10-outcome-dark.png`
- `docs/assets/full-flow/10-outcome-light.png`
- `docs/assets/full-flow/11-components-and-states.png`
- `docs/assets/full-flow/12-responsive-rules.png`
- `docs/assets/full-flow/agentguild-full-flow.pdf` — multi-page review export.
- `docs/assets/full-flow/README.md` — screen index, API status legend, and export instructions.

**Read only:**

- `docs/assets/agentguild-tasks.png`
- `docs/assets/agentguild-code-review.png`
- `docs/assets/agentguild-review-command-center.png`
- `docs/assets/agentguild-review-evidence-first.png`
- `docs/superpowers/specs/2026-07-06-full-product-flow-visual-design.md`
- `backend/internal/transport/rest/router.go`
- `backend/internal/transport/rest/openapi.yaml`
- `frontend/src/app/AppShell.tsx`
- `frontend/src/styles/tokens.css`

---

### Task 1: Establish Pencil Source, Page Map, and API Snapshot

**Files:**

- Create: `docs/assets/agentguild-full-flow.pen`
- Create: `docs/assets/full-flow/README.md`
- Read: `docs/superpowers/specs/2026-07-06-full-product-flow-visual-design.md`
- Read: `backend/internal/transport/rest/router.go`
- Read: `backend/internal/transport/rest/openapi.yaml`

**Interfaces:**

- Consumes: current REST route inventory and the approved ten-page screen specification.
- Produces: Pencil document roots named `00 Foundations`, `01 Components`, `02 Journey`, `03 API Coverage`, `04 Screens Dark`, `05 Screens Light`, and `06 Responsive`.

- [ ] **Step 1: Load the Pencil schema and web-app guidelines**

Call `mcp__pencil.get_editor_state` with `include_schema: true`, then list and load the single guideline appropriate for desktop web application design. Do not call any other Pencil tool before the schema is loaded.

Expected: the response contains the active `.pen` schema, batch-design rules, and supported variable/theme behavior.

- [ ] **Step 2: Create the design source and top-level page map**

Use `mcp__pencil.batch_design` with `filePath` set to:

```text
/Users/pengzhen/work/AgentGuild/docs/assets/agentguild-full-flow.pen
```

Create seven horizontally separated top-level frames with these exact names:

```text
00 Foundations
01 Components
02 Journey
03 API Coverage
04 Screens Dark
05 Screens Light
06 Responsive
```

Use at least 240px between top-level frames so document screenshots remain readable and selection is unambiguous.

- [ ] **Step 3: Record the API support snapshot in the design document**

Create rows for:

```text
Login — available
Onboarding persistence — planned
GitHub App CRUD — available
GitHub connection test — planned
Repository catalog — planned
Sync rule CRUD/preview — planned
Sync runs/conflicts — planned
Task list/detail — available
Execution detail — partial
Submission detail/diff — partial
Validation job detail — planned
Review read/rubric — available
Review comments/decision — auth fix
Outcome aggregation/Issue write-back — partial
Theme preference — partial
```

Use four distinct annotation chips, but do not use these chips inside user-facing screen frames.

- [ ] **Step 4: Create the export README**

Create `docs/assets/full-flow/README.md` with:

```markdown
# AgentGuild Full-Flow Design

Source: `../agentguild-full-flow.pen`

## API labels

- `available`: current route can support the design.
- `partial`: current route supports only part of the displayed information.
- `planned`: backend/domain work is required.
- `auth fix`: route exists but human session authentication must be corrected.

## Export set

- Journey and API coverage boards
- Ten canonical screens in dark and light themes
- Components/states board
- Responsive-rules board
- Multi-page PDF review copy

The API labels live in design annotations and are not part of the user interface.
```

- [ ] **Step 5: Verify structure and commit**

Run `mcp__pencil.snapshot_layout` with `maxDepth: 1`. Confirm all seven root frames exist, do not overlap, and have no clipped children.

Run:

```bash
git add docs/assets/agentguild-full-flow.pen docs/assets/full-flow/README.md
git commit -m "design: scaffold full-flow source and API map"
```

Expected: one commit containing only the Pencil source and design README.

---

### Task 2: Build Theme Variables and Reusable Components

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`
- Read: `frontend/src/styles/tokens.css`

**Interfaces:**

- Consumes: Pencil source roots from Task 1.
- Produces: theme collections `AgentGuild/Dark` and `AgentGuild/Light`; reusable shell, navigation, data, feedback, diff, and review components.

- [ ] **Step 1: Define semantic variables**

Create shared semantic variables rather than page-specific colors:

```text
color.bg.canvas
color.bg.surface
color.bg.elevated
color.border.default
color.border.strong
color.text.primary
color.text.secondary
color.text.muted
color.action.primary
color.action.primaryText
color.state.success
color.state.warning
color.state.danger
color.state.info
color.diff.addedBg
color.diff.deletedBg
color.focus
space.1 = 4
space.2 = 8
space.3 = 12
space.4 = 16
space.5 = 20
space.6 = 24
radius.sm = 6
radius.md = 8
radius.lg = 12
```

Dark values must use a near-black blue-gray canvas, one-level-lighter surfaces, cyan primary action, muted green/amber/red states, and low-saturation diff backgrounds. Light values must use a cool-gray canvas, white surfaces, accessible blue/cyan action, darker semantic states, and pale diff backgrounds.

- [ ] **Step 2: Build the global application shell**

Create reusable components:

```text
AppRail
TopBar
ContextBar
PageHeader
CommandSearch
ThemeSwitch
UserMenu
NotificationBadge
```

The shell must support both onboarding mode and daily-workbench mode without changing component geometry.

- [ ] **Step 3: Build data and feedback components**

Create:

```text
StatusChip
ApiStatusAnnotation
ProgressSteps
FilterBar
DenseTable
TaskRow
DetailPanel
Timeline
MetricCard
EmptyState
InlineError
GlobalAlert
ConfirmationDialog
Toast
SkeletonRow
```

`StatusChip` must always include text or an icon. `ApiStatusAnnotation` belongs only to notes frames.

- [ ] **Step 4: Build Git, validation, and review components**

Create:

```text
ProviderCard
RepositoryRow
SyncRuleBuilder
SyncResultSummary
CheckRow
FileTree
DiffToolbar
DiffLine
LineComment
RubricDimension
ReviewDecisionBar
OutcomeSummary
```

Use realistic Chinese content and sanitized sample identifiers:

```text
Billing Platform
billing-service
AG-192
Atlas v12
a1b2c3d
agentguild/ag-192
```

- [ ] **Step 5: Verify both themes**

Use `mcp__pencil.get_screenshot` on `01 Components` once with the dark theme and once with the light theme. Confirm:

- no geometry changes between themes;
- primary, success, warning, danger, and diff colors remain distinguishable;
- body text and controls meet visually credible WCAG AA contrast;
- no gradient-heavy decoration or oversized rounded cards.

- [ ] **Step 6: Commit**

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: add dual-theme foundations and components"
```

---

### Task 3: Design Login, Onboarding, and Git Integration

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`

**Interfaces:**

- Consumes: shell and component library from Task 2.
- Produces: screens `01 Login`, `02 Onboarding`, and `03 Git Integration` in dark and light themes.

- [ ] **Step 1: Compose the login screens**

Create `01-login-dark` and `01-login-light` at 1440px width with identical geometry.

Required content:

```text
AgentGuild
企业 Agent 任务治理平台
使用企业 OIDC 登录
本地开发登录
密码错误或未启用本地登录
```

Show the local login section as available only when configured. Add a design annotation:

```text
API · available — OIDC routes and local-login route exist; local-login OpenAPI documentation is missing.
```

- [ ] **Step 2: Compose onboarding screens**

Create `02-onboarding-dark` and `02-onboarding-light`.

Show the five steps:

```text
登录
Git 接入
仓库
同步
完成
```

Set `Git 接入` as current, `登录` as complete, and later steps as pending. Include save-and-continue behavior and the condition that a successful sync is required to complete onboarding.

Annotation:

```text
API · planned — server-side onboarding state does not exist.
```

- [ ] **Step 3: Compose Git integration screens**

Create `03-git-integration-dark` and `03-git-integration-light`.

Required provider states:

```text
GitHub — available
GitLab — 即将支持 / disabled
```

Required permissions:

```text
Contents — 只读
Issues — 读写
Checks — 只读
Metadata — 只读
```

Show configured state, update, and delete. The private key field must never reveal a stored value.

Annotations:

```text
API · available — GET/POST/DELETE /v1/github-app
API · planned — connection-test action
```

- [ ] **Step 4: Validate and commit**

Run `snapshot_layout` with `problemsOnly: true` for all six screen frames. Take one screenshot of the three dark frames and one screenshot of the three light frames. Fix clipping, inconsistent alignment, and theme drift.

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: add login onboarding and Git integration screens"
```

---

### Task 4: Design Repository Rules and Synchronization Results

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`

**Interfaces:**

- Consumes: ProviderCard, RepositoryRow, SyncRuleBuilder, SyncResultSummary.
- Produces: screens `04 Repository & Sync Rule` and `05 Sync Result` in both themes.

- [ ] **Step 1: Compose repository and rule screens**

Create `04-repository-sync-rule-dark` and `04-repository-sync-rule-light`.

Required repository data:

```text
billing-service — enabled — main — private
event-gateway — enabled — main — private
legacy-monolith — disabled — master — private
```

Required rule:

```text
包含标签: agent-ready
排除标签: security-hold, needs-product
Issue 状态: open
任务类型: code
默认优先级: P1
同步频率: 每 5 分钟
重复策略: 更新现有 Task
```

Include natural-language preview, save draft, preview, enable, pause, and impact confirmation.

Annotation:

```text
API · planned — repository catalog and sync-rule CRUD/preview.
```

- [ ] **Step 2: Compose sync-result screens**

Create `05-sync-result-dark` and `05-sync-result-light`.

Summary:

```text
新增 8
更新 3
忽略 5
冲突 2
失败 1
```

Table columns:

```text
Issue
Task
Result
Reason
Updated at
Action
```

Show one conflict, one ignored Issue, and one retryable platform failure. The conflict row offers retry, ignore, and pause rule; it does not offer editing the formal Task.

Annotation:

```text
API · planned — sync runs, conflicts, retry, and audit.
```

- [ ] **Step 3: Validate and commit**

Use `snapshot_layout(problemsOnly: true)` and inspect dark/light screenshots side by side. Confirm tables remain readable at 1440px and all planned controls are visibly annotated only in notes.

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: add repository rules and sync result screens"
```

---

### Task 5: Design Task Center and Execution Detail

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`
- Read: `frontend/src/features/tasks/TaskList.tsx`
- Read: `frontend/src/features/tasks/TaskDetail.tsx`

**Interfaces:**

- Consumes: current task and execution response fields plus dense workbench components.
- Produces: screens `06 Task Center` and `07 Execution Detail` in both themes.

- [ ] **Step 1: Compose task-center screens**

Create `06-task-center-dark` and `06-task-center-light`.

Required tabs:

```text
全部
待领取
进行中
待审核
已完成
异常
```

Required filters:

```text
仓库
类型
Agent
来源
同步状态
时间
```

Required columns:

```text
Task ID
标题
仓库
类型
Agent
状态
截止时间
来源
```

The selected task opens a right detail panel with source Issue, objective, acceptance criteria, active execution, and last sync status. Do not include claim or submit buttons.

Annotations:

```text
API · available — task list/detail core fields.
API · partial — source Issue, sync batch, and sync status.
```

- [ ] **Step 2: Compose execution-detail screens**

Create `07-execution-detail-dark` and `07-execution-detail-light`.

Required facts:

```text
Atlas v12
Lease 剩余 1h 42m
最近心跳 20 秒前
agentguild/ag-192
base a18d220
observed $0.067
self-reported $0.070
coverage partial
```

Timeline events:

```text
任务已领取
执行已开始
凭证已签发
运行测试
Commit 已推送
等待提交验证
```

Do not present `progress` as a reliable completion percentage. Use stage and event facts instead.

Annotation:

```text
API · partial — core Execution is readable; event timeline and revision list require new queries.
```

- [ ] **Step 3: Validate and commit**

Check both screen pairs with `snapshot_layout` and screenshots. Confirm URL/share state is represented in design notes and that no human-only mutation violates the product boundary.

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: add task center and execution detail screens"
```

---

### Task 6: Design Submission Validation and Review Workspace

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`
- Read: `frontend/src/features/reviews/ReviewPage.tsx`
- Read: `frontend/src/features/reviews/DiffViewer.tsx`
- Read: `frontend/src/features/reviews/RubricForm.tsx`

**Interfaces:**

- Consumes: submission/diff endpoints, validation-domain step model, review/rubric endpoints.
- Produces: screens `08 Submission Validation` and `09 Review Workspace` in both themes.

- [ ] **Step 1: Compose submission-validation screens**

Create `08-submission-validation-dark` and `08-submission-validation-light`.

Header facts:

```text
repo billing-service
branch agentguild/ag-192
base a18d220
commit a1b2c3d
validation attempt 1
```

Validation groups:

```text
Commit exists — passed
Branch matches — passed
Base is ancestor — passed
Build — passed
Public tests — 42/42
Hidden tests — 18/20
Security scan — passed
Change scope — warning
```

Show platform retry only for a platform error, not for failed code checks.

Annotations:

```text
API · partial — submission and diff exist.
API · planned — validation-job detail query and log detail.
```

- [ ] **Step 2: Compose review-workspace screens**

Create `09-review-workspace-dark` and `09-review-workspace-light`.

Layout at 1440px:

```text
left: file tree and reviewed-file progress
center: evidence summary, Diff, and line comments
right: automated checks, Rubric, summary, decision actions
```

Use six rubric dimensions totaling 100 points. Show hidden tests at `18/20` and a hard-gate warning. Disable “通过并评分” when a hard gate fails; keep “退回修改” available.

Annotations:

```text
API · available — review, diff, and active rubric reads.
API · auth fix — comments and decision currently use bearer-only authentication and must support human session auth.
```

- [ ] **Step 3: Validate and commit**

Use `snapshot_layout(problemsOnly: true)` for all four frames. Use screenshots to verify Diff readability, comment anchoring, Rubric density, disabled-decision clarity, and equivalent dark/light geometry.

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: add validation and review workspace screens"
```

---

### Task 7: Design Outcome, Journey, Components/States, and Responsive Boards

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`

**Interfaces:**

- Consumes: all prior components/screens and the approved responsibility flow.
- Produces: screen `10 Outcome` in both themes plus journey, components/state, and responsive boards.

- [ ] **Step 1: Compose outcome screens**

Create `10-outcome-dark` and `10-outcome-light`.

Show the accepted path as the primary state:

```text
Task completed
Execution accepted
Review score 89/100
GitHub Issue write-back succeeded
Reputation +12
Experience candidate created
```

Include a clearly separated revision-requested panel:

```text
2 unresolved review comments
Agent preparing next revision
No new revision submitted yet
```

Annotations:

```text
API · partial — reputation and experience data exist.
API · planned — outcome aggregation, Issue write-back state, and audit timeline.
```

- [ ] **Step 2: Build the end-to-end journey board**

Create responsibility lanes for:

```text
人类治理
平台编排
Agent 执行
```

Connect:

```text
登录 → Git 接入 → 仓库与规则 → 同步预览 → 任务中心 → Agent 领取与执行 → 提交验证 → 人工审核 → 通过/返工 → Issue/声望/经验闭环
```

The board must show that humans configure and review, while Agent-only actions remain off the human interaction path.

- [ ] **Step 3: Build components-and-states board**

Show dark and light variants for:

```text
default
hover
focus
selected
disabled
loading
empty
success
warning
danger
permission expired
sync conflict
validation hard gate
```

Include sanitized copy and keyboard/focus annotations.

- [ ] **Step 4: Build responsive-rules board**

Create wireframes for:

```text
1440 — three-column workbench
1280 — compact three-column workbench
1024 — two columns with detail drawer
768 — single main column, route-based detail, bottom review drawer
```

Document that below 720px is read-only fallback scope, not a separate high-fidelity target.

- [ ] **Step 5: Validate and commit**

Use `snapshot_layout(problemsOnly: true)` for `02 Journey`, `06 Responsive`, and the outcome frames. Take screenshots of each completed board.

```bash
git add docs/assets/agentguild-full-flow.pen
git commit -m "design: complete outcome journey states and responsive boards"
```

---

### Task 8: Visual QA, Export, and Final Index

**Files:**

- Modify: `docs/assets/agentguild-full-flow.pen`
- Modify: `docs/assets/full-flow/README.md`
- Create: all PNG and PDF files listed in the File Map.

**Interfaces:**

- Consumes: all final Pencil frames from Tasks 1–7.
- Produces: verified user-facing PNGs, documentation boards, and a multi-page PDF.

- [ ] **Step 1: Run structural QA**

Run `mcp__pencil.snapshot_layout` with `problemsOnly: true` for every top-level root. Fix all unintended overlap, clipping, zero-size layers, and off-canvas children.

Expected: no layout problems remain. Intentional Diff overlays and comment popovers must be named with `Intentional Overlay` so they are distinguishable during review.

- [ ] **Step 2: Run visual QA**

Invoke the `visual-verdict` skill for:

```text
all ten dark/light screen pairs
task-center screen against docs/assets/agentguild-tasks.png
review dark screen against docs/assets/agentguild-review-command-center.png
review light screen against docs/assets/agentguild-review-evidence-first.png
```

Acceptance:

- dark/light geometry matches;
- hierarchy and density remain consistent with existing AgentGuild references;
- no unreadable text, low-contrast status, accidental clipping, or excessive decoration;
- review Diff remains the visual center;
- planned API annotations do not appear in user-facing frames.

- [ ] **Step 3: Export PNG screens and boards**

Use `mcp__pencil.export_nodes` with:

```json
{
  "filePath": "/Users/pengzhen/work/AgentGuild/docs/assets/agentguild-full-flow.pen",
  "format": "png",
  "outputDir": "/Users/pengzhen/work/AgentGuild/docs/assets/full-flow",
  "scale": 2
}
```

Export the exact frame IDs corresponding to all filenames in the File Map. Rename exported node-ID filenames to the specified semantic filenames only after verifying the frame-to-file mapping.

- [ ] **Step 4: Export the review PDF**

Use `mcp__pencil.export_nodes` with `format: "pdf"` and this page order:

```text
Journey
API Coverage
Components and States
01 Login Dark/Light
02 Onboarding Dark/Light
03 Git Integration Dark/Light
04 Repository & Sync Rule Dark/Light
05 Sync Result Dark/Light
06 Task Center Dark/Light
07 Execution Detail Dark/Light
08 Submission Validation Dark/Light
09 Review Workspace Dark/Light
10 Outcome Dark/Light
Responsive Rules
```

Write to `docs/assets/full-flow/agentguild-full-flow.pdf`.

- [ ] **Step 5: Verify export inventory**

Run:

```bash
find docs/assets/full-flow -maxdepth 1 -type f -print | sort
```

Expected:

- 20 canonical screen PNGs;
- 4 board PNGs;
- 1 PDF;
- 1 README;
- no node-ID-only duplicate exports.

- [ ] **Step 6: Update the README with final frame IDs and API caveats**

Add:

- final semantic filename → Pencil frame ID mapping;
- current interface gaps;
- review auth mismatch;
- OpenAPI coverage gap;
- export date and Pencil source path.

- [ ] **Step 7: Final repository checks**

Run:

```bash
git status --short
git diff --check
```

Confirm unrelated existing changes remain untouched. Verify only the `.pen`, `docs/assets/full-flow/*`, and the already approved design documentation are staged for this work.

- [ ] **Step 8: Commit**

```bash
git add docs/assets/agentguild-full-flow.pen docs/assets/full-flow
git commit -m "design: deliver AgentGuild full-flow dual-theme mockups"
```

Expected: final design-delivery commit with editable source, 20 screens, four boards, PDF, and README.

## Follow-up Software Plans

The following are intentionally outside this visual-design execution plan and should be planned independently after the design is accepted:

1. OpenAPI completeness and human review session-auth correction.
2. Repository catalog and GitHub App connection-test APIs.
3. Issue sync rules, preview, run history, conflict handling, and Issue↔Task mapping.
4. Validation-job detail query and execution-event/revision timelines.
5. Outcome aggregation, GitHub Issue write-back, onboarding persistence, and optional cross-device theme preference.
