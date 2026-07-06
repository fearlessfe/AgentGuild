# AgentGuild Full Product Flow HTML Design Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a code-native, editable HTML design gallery and export the approved AgentGuild journey, API coverage board, 20 dark/light desktop screens, component/state board, responsive board, and multi-page PDF.

**Architecture:** A static renderer reads screen definitions from `screens.js`, composes reusable HTML components in `app.js`, and applies semantic theme tokens from `styles.css`. `render-mockups.mjs` uses the repository’s Playwright installation to open each `screen`/`theme` route at 1440px, capture 2x PNGs, and print the full gallery as PDF.

**Tech Stack:** Semantic HTML, CSS custom properties, vanilla JavaScript, Node.js, Playwright Chromium, in-app browser visual verification.

## Global Constraints

- Human UI must not expose Agent-only claim, execution, credential, heartbeat, or submission operations.
- Git provider UI is generic; GitHub is enabled and GitLab is disabled.
- Task synchronization is rule-driven with preview, pause, retry, and conflict handling.
- Exactly ten canonical screens are rendered in both themes with identical geometry.
- Canonical desktop viewport is 1440×1024; exports use device scale factor 2.
- Responsive guidance covers 1280px, 1024px, and 768px.
- No real credentials, tokens, private keys, tenant IDs, or personal data.
- API feasibility appears only in design annotations and uses `available`, `partial`, `planned`, and `auth fix`.
- Existing React application code is read-only for this design task.
- Unrelated repository changes must remain untouched.

## Deliverables

- `design/agentguild-full-flow/index.html`
- `design/agentguild-full-flow/styles.css`
- `design/agentguild-full-flow/screens.js`
- `design/agentguild-full-flow/app.js`
- `design/render-mockups.mjs`
- `docs/assets/full-flow/README.md`
- `docs/assets/full-flow/00-full-flow-journey.png`
- `docs/assets/full-flow/00-api-coverage.png`
- `docs/assets/full-flow/01-login-{dark,light}.png`
- `docs/assets/full-flow/02-onboarding-{dark,light}.png`
- `docs/assets/full-flow/03-git-integration-{dark,light}.png`
- `docs/assets/full-flow/04-repository-sync-rule-{dark,light}.png`
- `docs/assets/full-flow/05-sync-result-{dark,light}.png`
- `docs/assets/full-flow/06-task-center-{dark,light}.png`
- `docs/assets/full-flow/07-execution-detail-{dark,light}.png`
- `docs/assets/full-flow/08-submission-validation-{dark,light}.png`
- `docs/assets/full-flow/09-review-workspace-{dark,light}.png`
- `docs/assets/full-flow/10-outcome-{dark,light}.png`
- `docs/assets/full-flow/11-components-and-states.png`
- `docs/assets/full-flow/12-responsive-rules.png`
- `docs/assets/full-flow/agentguild-full-flow.pdf`

---

### Task 1: Scaffold the Code-Native Gallery and API Board

**Files:**

- Create: `design/agentguild-full-flow/index.html`
- Create: `design/agentguild-full-flow/styles.css`
- Create: `design/agentguild-full-flow/screens.js`
- Create: `design/agentguild-full-flow/app.js`
- Create: `docs/assets/full-flow/README.md`

**Interfaces:**

- Produces: `renderScreen(screenId, theme)` and routes `?screen=<id>&theme=<dark|light>`.
- Screen IDs: `journey`, `api-coverage`, `login`, `onboarding`, `git-integration`, `repository-sync-rule`, `sync-result`, `task-center`, `execution-detail`, `submission-validation`, `review-workspace`, `outcome`, `components-states`, `responsive-rules`, `gallery`.

- [ ] Create the HTML entry with `#app`, an accessible title, and module script loading `app.js`.
- [ ] Define the complete screen registry and sanitized shared data in `screens.js`.
- [ ] Implement route parsing and `renderScreen(screenId, theme)` in `app.js`; unknown IDs render a visible error with valid IDs.
- [ ] Define foundation CSS for 1440×1024 `.screen`, annotation boards, print page breaks, and temporary neutral tokens.
- [ ] Render the API coverage board with all 15 capability rows from the approved spec.
- [ ] Create `docs/assets/full-flow/README.md` with source path, route format, API legend, export inventory, and the note that annotations are not user UI.
- [ ] Open `index.html?screen=api-coverage&theme=light` in the in-app browser and verify all rows are visible without horizontal clipping.
- [ ] Commit with `design: scaffold HTML full-flow gallery`.

### Task 2: Build Semantic Themes and Reusable Components

**Files:**

- Modify: `design/agentguild-full-flow/styles.css`
- Modify: `design/agentguild-full-flow/app.js`

**Interfaces:**

- Produces reusable renderers: `appShell`, `pageHeader`, `statusChip`, `apiNote`, `steps`, `denseTable`, `detailPanel`, `timeline`, `emptyState`, `providerCard`, `checkRow`, `diffViewer`, `rubricForm`, `outcomeSummary`.

- [ ] Add dark and light semantic CSS tokens for canvas, surface, elevated, borders, text, action, success, warning, danger, info, focus, and diff backgrounds.
- [ ] Add shared spacing on a 4px base, 6/8/12px radii, 40–44px dense rows, system Chinese fonts, and monospace identifiers.
- [ ] Implement reusable renderers without screen-specific color literals.
- [ ] Render the components/state board with default, hover, focus, selected, disabled, loading, empty, success, warning, danger, permission-expired, sync-conflict, and hard-gate states.
- [ ] Verify dark/light geometry by switching only the `theme` query parameter at 1440×1024.
- [ ] Commit with `design: add HTML themes and component system`.

### Task 3: Design Login, Onboarding, and Git Integration

**Files:**

- Modify: `design/agentguild-full-flow/screens.js`
- Modify: `design/agentguild-full-flow/app.js`
- Modify: `design/agentguild-full-flow/styles.css`

- [ ] Implement login with OIDC primary action, conditional local-login form, error message, and `API · available` note.
- [ ] Implement five-step onboarding with Git connection current, login complete, server-persisted progress marked `API · planned`.
- [ ] Implement Git provider selection, GitHub permission list, configured state, hidden private key, disabled GitLab card, and connection-test planned note.
- [ ] Verify the three pages in both themes at 1440×1024; no geometry may change between themes.
- [ ] Commit with `design: add login onboarding and Git screens`.

### Task 4: Design Repository Rules and Sync Results

**Files:**

- Modify: `design/agentguild-full-flow/screens.js`
- Modify: `design/agentguild-full-flow/app.js`
- Modify: `design/agentguild-full-flow/styles.css`

- [ ] Implement repository search/selection with billing-service, event-gateway, and disabled legacy-monolith.
- [ ] Implement rule fields for included/excluded labels, Issue status, task type, priority, frequency, and duplicate strategy.
- [ ] Implement natural-language preview, save draft, preview, enable, pause, and impact confirmation.
- [ ] Implement sync summary counts 8/3/5/2/1 and result table with conflict, ignored, and retryable failure examples.
- [ ] Ensure conflict actions are retry, ignore, and pause; no formal Task editing control.
- [ ] Mark both pages `API · planned` and verify both themes.
- [ ] Commit with `design: add repository rules and sync results`.

### Task 5: Design Task Center and Execution Detail

**Files:**

- Modify: `design/agentguild-full-flow/screens.js`
- Modify: `design/agentguild-full-flow/app.js`
- Modify: `design/agentguild-full-flow/styles.css`

- [ ] Implement task tabs, repository/type/Agent/source/sync/time filters, dense grouped list, and right detail panel.
- [ ] Show available task fields separately from partial Issue/sync fields in notes.
- [ ] Implement execution facts for Atlas v12, lease, heartbeat, branch, base commit, observed/self-reported cost, and partial coverage.
- [ ] Implement event timeline without using progress as a reliable completion percentage.
- [ ] Verify both pages against `docs/assets/agentguild-tasks.png` for density and hierarchy.
- [ ] Commit with `design: add task center and execution detail`.

### Task 6: Design Submission Validation and Review Workspace

**Files:**

- Modify: `design/agentguild-full-flow/screens.js`
- Modify: `design/agentguild-full-flow/app.js`
- Modify: `design/agentguild-full-flow/styles.css`

- [ ] Implement validation facts, eight check groups, attempt metadata, hidden tests 18/20, and platform-only retry state.
- [ ] Mark submission/diff partial and validation-job detail planned.
- [ ] Implement three-column review: file tree, evidence/Diff/comments, checks/Rubric/decision.
- [ ] Use six dimensions totaling 100; disable accept when hard gate fails and leave revision request active.
- [ ] Mark review reads available and comments/decision `auth fix`.
- [ ] Verify dark review against command-center reference and light review against evidence-first reference.
- [ ] Commit with `design: add validation and review workspaces`.

### Task 7: Design Outcome, Journey, and Responsive Boards

**Files:**

- Modify: `design/agentguild-full-flow/screens.js`
- Modify: `design/agentguild-full-flow/app.js`
- Modify: `design/agentguild-full-flow/styles.css`

- [ ] Implement accepted outcome with score 89/100, Issue write-back, reputation +12, experience candidate, and audit summary.
- [ ] Include revision-requested panel with two comments and no new revision yet.
- [ ] Implement human/platform/Agent responsibility-lane journey covering the entire loop.
- [ ] Implement 1440, 1280, 1024, and 768 responsive wireframes and behavioral notes.
- [ ] Verify journey connections and responsive boards in the in-app browser.
- [ ] Commit with `design: complete outcome journey and responsive boards`.

### Task 8: Export and Final Visual QA

**Files:**

- Create: `design/render-mockups.mjs`
- Modify: `docs/assets/full-flow/README.md`
- Create: all PNG/PDF deliverables.

- [ ] Implement a Playwright renderer using `createRequire` rooted at `frontend/package.json`.
- [ ] For every canonical page and theme, open the file URL with query parameters, set viewport 1440×1024 and device scale factor 2, wait for fonts, and screenshot `.screen`.
- [ ] Export journey, API coverage, components/states, and responsive boards.
- [ ] Open `gallery` with print media and export one multi-page PDF.
- [ ] Run the renderer and verify exactly 20 screen PNGs, 4 board PNGs, 1 PDF, and 1 README.
- [ ] Use in-app browser screenshots and the visual-verdict skill to review all dark/light pairs and reference comparisons.
- [ ] Fix clipping, contrast, hierarchy, theme geometry drift, and annotation leakage.
- [ ] Run `git diff --check` and confirm application source remains untouched.
- [ ] Commit with `design: deliver full-flow dual-theme mockups`.

## Follow-up Software Plans

1. OpenAPI completeness and review session-auth correction.
2. Repository catalog and GitHub App connection test.
3. Issue sync rules, previews, run history, conflicts, and Issue↔Task mapping.
4. Validation detail and execution event/revision queries.
5. Outcome aggregation, Issue write-back, onboarding persistence, and optional cross-device theme preference.
