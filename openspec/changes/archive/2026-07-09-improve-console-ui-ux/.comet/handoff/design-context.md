# Comet Design Handoff

- Change: improve-console-ui-ux
- Phase: design
- Mode: compact
- Context hash: ebddd226f391e9db8a30b0da8cc22fe346eafa9453f0f89e345e2249396bb436

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/improve-console-ui-ux/proposal.md

- Source: openspec/changes/improve-console-ui-ux/proposal.md
- Lines: 1-35
- SHA256: 97698f973b9eb585476b40bebfef4ca1bc84189dd1a6eeaa336665c7f18a0476

```md
## Why

AgentGuild console already has the right product direction for an internal governance workbench, but the current UI has several usability defects that weaken the core human review loop. The most severe issue is the review diff becoming unreadable through character-level wrapping; mobile task and agent lists also hide important information without a clear interaction model.

This change turns the UI/UX review findings into a focused frontend improvement pass so the task, agent, and review workspaces feel reliable, readable, and professionally consistent across desktop and small mobile viewports.

## What Changes

- Improve the review workspace diff viewer so code remains readable on desktop and mobile.
- Adapt task and agent list presentation for narrow viewports, avoiding silent clipping of core information.
- Replace character-symbol structural icons in shell/navigation/provider surfaces with a consistent SVG icon system.
- Raise mobile interaction quality for navigation, icon buttons, tabs, and key controls with clearer hit targets and focus states.
- Strengthen loading, empty, submitting, and error feedback around core task, agent, and review workflows.
- Add frontend verification for desktop and mobile viewports to guard against diff readability regressions, hidden content, and page-level horizontal overflow.

## Capabilities

### New Capabilities
- `console-ui-ux`: Frontend console usability requirements for review diff readability, responsive workbench layouts, consistent iconography, interaction targets, and feedback states.

### Modified Capabilities
- None. This change does not alter backend business requirements or existing task/review/agent state-machine semantics.

## Impact

- Affected frontend areas:
  - `frontend/src/app`
  - `frontend/src/ui`
  - `frontend/src/styles`
  - `frontend/src/features/tasks`
  - `frontend/src/features/reviews`
  - `frontend/src/features/agents`
  - `frontend/e2e`
- No backend API, database, or transport changes are intended.
- A frontend icon dependency may be added if the chosen SVG icon library is not already available.
```

## openspec/changes/improve-console-ui-ux/design.md

- Source: openspec/changes/improve-console-ui-ux/design.md
- Lines: 1-99
- SHA256: ba7c2bc078ad4878910b5c8092fc196665dcc08b1a14d2732865f81c9579cb58

[TRUNCATED]

```md
## Context

AgentGuild is an internal agent task governance console. The frontend already follows the right broad pattern for this product class: dark, dense, utilitarian workbench screens with persistent navigation, compact tables, and split panes for task/review details.

The current implementation has several user-facing quality gaps found during a `ui-ux-pro-max` review and Playwright visual inspection:

- The review diff pane wraps code into character-level columns in both desktop and mobile views, making the core human review task unreliable.
- Task and agent tables avoid page-level horizontal overflow on mobile by clipping content inside cards, which hides core information without an obvious interaction.
- Shell, navigation, empty states, and provider surfaces use character symbols as structural icons, reducing visual consistency and theme control.
- Several interactive targets are below the 44px mobile target guideline.
- Loading and mutation feedback are mostly textual and inconsistent across core workflows.

## Goals / Non-Goals

**Goals:**

- Restore code readability in the review workspace across desktop and 375px mobile viewports.
- Provide mobile-appropriate task and agent list presentations that expose core fields without silent clipping.
- Standardize structural iconography on a single SVG icon family.
- Improve mobile hit targets, focus states, and interaction feedback for core controls.
- Add targeted frontend verification for the UI/UX regressions identified in the review.

**Non-Goals:**

- No backend API, database, or domain state-machine changes.
- No complete redesign of the brand, theme, information architecture, or product copy.
- No replacement of the existing React Router / TanStack Query frontend architecture.
- No change to who can create, claim, submit, or review tasks.

## Decisions

### Decision 1: Fix diff readability with container-level scrolling and responsive mode defaults

The diff viewer will preserve code lines instead of forcing arbitrary word breaking. Desktop split mode should keep readable columns and allow the diff pane to scroll horizontally when needed. Mobile review will default to unified mode below the mobile breakpoint because it gives reviewers the most readable single-column presentation while keeping the existing mode controls available.

Alternatives considered:

- Continue wrapping code aggressively: rejected because it destroys the primary review workflow.
- Force only unified mode everywhere: rejected because desktop reviewers often benefit from split comparison.
- Introduce a virtualized code editor component: deferred because it is larger than this UX stabilization pass.

### Decision 2: Use responsive list/card patterns for mobile workbench data

Task and agent list screens will keep dense tables on desktop, but narrow viewports will switch to compact item cards for primary scanning. Mobile cards are preferred over scroll wrappers because they expose title/status/repo/owner/action without relying on hidden columns or horizontal gestures.

Alternatives considered:

- Keep the current clipped tables: rejected because users cannot discover hidden information.
- Use horizontal scrolling only: acceptable for secondary dense data, but weaker for primary mobile scanning.
- Build a full separate mobile app shell: out of scope.

### Decision 3: Standardize structural icons with an SVG icon set

The shell navigation, topbar controls, provider logos, and empty-state icons will use a consistent SVG icon set. `lucide-react` is the preferred dependency because it fits the current React web stack, is lightweight, and aligns with existing frontend guidance. Icons remain decorative when paired with visible/ARIA labels.

Alternatives considered:

- Keep Unicode symbols: rejected due to inconsistent rendering and limited token control.
- Inline custom SVGs: rejected because it increases maintenance cost and risks inconsistent stroke style.
- Use multiple icon packs: rejected because it weakens visual cohesion.

### Decision 4: Establish a mobile interaction baseline in CSS primitives

The existing `Button`, `ButtonLink`, `.icon-btn`, `.rail-item`, `.tab`, select, and input styles will preserve desktop density while applying larger targets and clearer spacing at the mobile breakpoint. Focus-visible styles must remain visible in both themes.

Alternatives considered:

- Increase all controls globally to 44px: rejected because the desktop product intentionally uses high-density workbench layout.
- Leave desktop and mobile identical: rejected because current mobile hit targets are too small.

### Decision 5: Verify the UX baseline with focused Playwright checks

Existing e2e coverage will be extended with targeted checks for review diff readability, mobile task/agent presentation, no page-level horizontal overflow, and basic visual availability of key controls. The verification should remain small and stable; it should not become pixel-perfect snapshot churn for every page.

Alternatives considered:

- Rely on manual screenshots: rejected because the previous issue is easy to regress.
- Add broad screenshot baselines for all pages: deferred because the current goal is targeted UX risk reduction.

## Risks / Trade-offs
```

Full source: openspec/changes/improve-console-ui-ux/design.md

## openspec/changes/improve-console-ui-ux/tasks.md

- Source: openspec/changes/improve-console-ui-ux/tasks.md
- Lines: 1-37
- SHA256: be45c2951b95917c6da4dc1a4537897fdc532a932cde4e5d0cbfa43a3ee1a566

```md
## 1. Verification Baseline

- [ ] 1.1 Add or update Playwright helpers to measure mobile page-level horizontal overflow and key control visibility.
- [ ] 1.2 Add a regression check that captures the current unreadable diff wrapping on desktop and 375px mobile viewports, then make it pass with the implementation.
- [ ] 1.3 Add mobile checks for task and agent list core information visibility.

## 2. Review Diff Readability

- [ ] 2.1 Update review diff CSS so code line content preserves readable text and avoids character-level wrapping.
- [ ] 2.2 Add explicit diff-region overflow behavior that does not create page-level horizontal scrolling.
- [ ] 2.3 Improve mobile review diff behavior by defaulting to the most readable mode or presenting split mode inside an obvious scroll container.
- [ ] 2.4 Verify line comments and diff mode toggles remain usable after the layout change.

## 3. Mobile Task And Agent Lists

- [ ] 3.1 Adapt the task list for mobile so each item exposes id, title, status, and repository or publisher context without silent clipping.
- [ ] 3.2 Adapt the agent list for mobile so each item exposes name, status, owner or team context, and navigation affordance without silent clipping.
- [ ] 3.3 Preserve dense table behavior and existing filtering behavior on desktop.

## 4. Icon And Interaction Baseline

- [ ] 4.1 Add the selected SVG icon dependency if needed and replace structural Unicode icons in the rail, topbar, empty states, and provider cards.
- [ ] 4.2 Ensure icon-only interactive controls have accessible names and inherit semantic theme colors.
- [ ] 4.3 Apply mobile hit-target improvements for rail items, icon buttons, tabs, selects, and primary actions while preserving desktop density.
- [ ] 4.4 Confirm focus-visible states remain clear in dark and light themes.

## 5. Feedback States

- [ ] 5.1 Improve loading states for task, agent, and review regions that currently render plain text or sparse placeholders.
- [ ] 5.2 Ensure review decision actions and comment submission controls communicate pending state and prevent duplicate submission.
- [ ] 5.3 Review empty and error states for the affected routes and align them with the shared UI primitives.

## 6. Final Verification

- [ ] 6.1 Run frontend build and unit tests.
- [ ] 6.2 Run targeted Playwright checks for task, agent, and review routes on desktop and mobile viewports.
- [ ] 6.3 Capture or inspect screenshots for the changed core routes and adjust layout issues found during visual QA.
```

## openspec/changes/improve-console-ui-ux/specs/console-ui-ux/spec.md

- Source: openspec/changes/improve-console-ui-ux/specs/console-ui-ux/spec.md
- Lines: 1-77
- SHA256: 90f15da42387cf58e179ca6fd7119de686c2f4c145bcf4ae2f41634411a74465

```md
## ADDED Requirements

### Requirement: Review diff remains readable
The console SHALL render review diffs so code lines remain readable in desktop and mobile viewports. Diff content MUST NOT wrap into character-by-character columns, and any overflow needed to preserve code readability MUST be contained within the diff region rather than causing page-level horizontal overflow.

#### Scenario: Desktop reviewer reads split diff
- **WHEN** a reviewer opens a review detail page on a desktop viewport
- **THEN** the split diff presents old and new code columns with readable line content
- **AND** long code lines are preserved through diff-region scrolling or another explicit readable presentation

#### Scenario: Mobile reviewer reads diff
- **WHEN** a reviewer opens a review detail page at a 375px-wide viewport
- **THEN** the diff remains readable without character-level wrapping
- **AND** the page itself does not require horizontal scrolling

### Requirement: Mobile workbench lists expose core information
The console SHALL adapt task and agent list views for narrow mobile viewports so users can read core item information without silent clipping. Core information includes item identity, title or name, status, and the most important ownership or repository context for that list.

#### Scenario: Mobile user scans tasks
- **WHEN** a user opens the task list at a 375px-wide viewport
- **THEN** each visible task exposes its id, title, status, and repository or publisher context
- **AND** hidden table columns are not silently clipped by the card boundary

#### Scenario: Mobile user scans agents
- **WHEN** a user opens the agent list at a 375px-wide viewport
- **THEN** each visible agent exposes its name, status, owner or team context, and navigation affordance
- **AND** hidden table columns are not silently clipped by the card boundary

### Requirement: Structural icons are consistent and token-controllable
The console SHALL use a consistent SVG icon family for structural navigation, topbar controls, provider cards, empty states, and repeated action affordances. Unicode character symbols MUST NOT be used as primary structural icons in these surfaces.

#### Scenario: User views the application shell
- **WHEN** a user opens any authenticated console route
- **THEN** navigation and topbar iconography uses the chosen SVG icon family
- **AND** each icon-only interactive control has an accessible name

#### Scenario: Theme changes
- **WHEN** the user switches between dark and light themes
- **THEN** structural icons inherit semantic color tokens and remain visually consistent

### Requirement: Mobile controls meet interaction baseline
The console SHALL provide mobile-friendly hit targets for primary navigation, icon buttons, tabs, and form controls. On mobile viewports, primary interactive controls MUST provide a practical hit area of at least 44px by 44px unless the control is non-essential and paired with a larger interactive parent.

#### Scenario: Keyboard user navigates controls
- **WHEN** a keyboard user tabs through shell controls, filters, and primary actions
- **THEN** the current focus is visibly indicated in both dark and light themes

#### Scenario: Touch user operates mobile navigation
- **WHEN** a touch user opens the console at a mobile viewport
- **THEN** primary navigation and topbar controls have touch-friendly hit areas and do not require precision taps

### Requirement: Core workflows communicate loading and mutation states
The console SHALL provide clear feedback while core task, agent, and review workflows load or submit data. Buttons that trigger asynchronous mutations MUST prevent accidental duplicate submission while pending.

#### Scenario: Review decision is submitting
- **WHEN** a reviewer submits an accept, reject, or revision-request decision
- **THEN** the selected action communicates pending state
- **AND** decision actions cannot be submitted repeatedly during the same pending request

#### Scenario: Agent mutation is submitting
- **WHEN** a user registers an Agent or submits an Agent management action
- **THEN** the selected action communicates pending state
- **AND** duplicate submission is prevented until the request succeeds or fails
- **AND** errors provide a visible recovery path

#### Scenario: Core route loads data
- **WHEN** task, agent, or review data is loading for longer than an immediate render
- **THEN** the console presents a loading, skeleton, or progress state in the affected region rather than appearing frozen or blank

### Requirement: Frontend verification protects responsive UX
The frontend test suite SHALL include targeted checks for the UI/UX baseline introduced by this change.

#### Scenario: Regression tests run for console UX
- **WHEN** frontend verification runs
- **THEN** tests cover desktop and mobile review diff readability
- **AND** tests cover mobile task and agent list visibility
- **AND** tests check that key routes do not create page-level horizontal overflow at mobile width
```

