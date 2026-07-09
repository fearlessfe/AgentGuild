---
comet_change: improve-console-ui-ux
role: technical-design
canonical_spec: openspec
---

# Improve Console UI/UX Technical Design

## Context

AgentGuild is an internal agent task governance console. Its current frontend direction is sound for the product: dark, dense, work-focused screens with persistent navigation, compact tables, and split workspaces. The UI/UX review identified a small set of high-impact problems that undermine that direction:

- Review diffs can wrap into character-level columns, making the core human review task unreliable.
- Task and Agent tables avoid page-level overflow on mobile by clipping content inside cards, hiding important fields.
- Structural icons in the shell and repeated UI surfaces use Unicode characters rather than a consistent SVG icon system.
- Several mobile controls are below the practical 44px by 44px hit-area baseline.
- Loading, submitting, empty, and error states are inconsistent across task, Agent, and review workflows.

The OpenSpec delta `console-ui-ux` is the canonical requirement source. This document defines the technical approach for implementing it.

## Goals

- Preserve desktop workbench density while fixing the highest-risk usability failures.
- Make review diffs readable on desktop and 375px mobile viewports.
- Make task and Agent mobile lists readable without silent clipping.
- Replace structural Unicode icons with a consistent SVG icon set.
- Improve mobile hit targets and focus states through scoped primitive/style changes.
- Add targeted tests that protect the known UI/UX regressions.

## Non-Goals

- No backend API, database, transport, or domain state-machine changes.
- No complete redesign of the product shell, brand, routing, or information architecture.
- No replacement of React Router, TanStack Query, or the existing CSS token system.
- No broad formal design-system extraction beyond the primitives needed for this change.

## Selected Approach

Use a focused workbench UX pass:

1. Add targeted Playwright checks for the known regressions.
2. Fix the review diff layout and mobile default mode.
3. Add mobile card presentations for task and Agent lists while preserving desktop tables.
4. Introduce `lucide-react` for structural icons and replace Unicode icon usage in the affected surfaces.
5. Improve mobile hit targets and feedback states through existing UI primitives and breakpoint-scoped CSS.
6. Verify through build, unit tests, targeted e2e, and visual inspection screenshots.

This approach avoids a broad redesign while still correcting the issues that make the console feel unreliable.

## Key Decisions

### Review Diff

Desktop split mode remains available because it is useful for code review. Code text should preserve readable lines rather than using aggressive wrapping. The diff region may scroll horizontally, but the page itself must not.

Mobile review defaults to Unified mode below the mobile breakpoint. Split mode remains available through the existing controls, but Unified is the primary readable path on small screens.

Implementation direction:

- Update diff table/pre styles to avoid character-level wrapping.
- Add an explicit overflow container around diff content.
- Use viewport-aware default mode for `DiffViewer`.
- Keep line selection, inline comment rows, and mode toggles intact.

### Mobile Task And Agent Lists

Desktop keeps dense table behavior. Mobile switches to compact cards for primary list scanning.

Task cards should expose:

- Task id
- Title
- Status
- Repository or publisher context
- Deadline or other existing high-value metadata when space allows

Agent cards should expose:

- Agent name
- Status
- Owner or team context
- Last seen or scope summary when space allows
- Clear navigation affordance

The mobile card views should reuse the same source data and filtering state as the desktop table/list implementations. They should not introduce separate data-fetching paths.

### Icon System

Use `lucide-react` as the single SVG icon family for structural icons introduced by this change. Import only the icons used by each file.

Initial replacement targets:

- `Rail`
- `Topbar`
- shared empty-state icon surfaces
- provider cards
- obvious repeated action affordances in the affected routes

Icon-only interactive controls must keep accessible names. Decorative icons inside labeled controls should remain `aria-hidden`.

### Interaction Baseline

Preserve desktop density. Apply larger hit areas at mobile breakpoints for:

- rail items
- rail toggle
- topbar icon buttons
- tabs
- selects and inputs
- primary action buttons

The practical target is at least 44px by 44px for primary mobile controls. Focus-visible styles must remain clear in both dark and light themes.

### Feedback States

Core routes should communicate loading and mutation states in the affected region rather than relying on sparse text or appearing static.

Implementation should cover:

- review decision buttons
- comment submission controls
- Agent registration and Agent management actions
- task, Agent, and review loading regions
- empty and error states aligned with shared primitives

Mutation controls must prevent duplicate submission while pending and expose a visible recovery path on failure.

## Testing Strategy

Use targeted tests rather than broad snapshot churn.

Add Playwright coverage for:

- review diff readability on desktop
- review diff readability on 375px mobile
- no page-level horizontal overflow on mobile task, Agent, and review routes
- mobile task list core information visibility
- mobile Agent list core information visibility
- key shell/icon controls remaining visible and accessible

Run:

- `cd frontend && npm run build`
- `cd frontend && npm test -- --run`
- targeted Playwright specs for task, Agent, and review routes

Inspect screenshots for `/tasks`, `/agents`, and `/reviews/:id` at desktop and 375px mobile widths before completion.

## Risks And Mitigations

- Adding `lucide-react` increases dependency surface. Mitigate with tree-shaken named imports and no mixed icon packs.
- Mobile cards duplicate presentation logic. Mitigate by sharing source data and leaving filtering/query code unchanged.
- Diff scrolling can be awkward on mobile. Mitigate by defaulting mobile to Unified mode.
- CSS primitive changes can affect legacy screens. Mitigate by scoping hit target changes to mobile breakpoints and testing core routes.
- Tests that assert visual details can become brittle. Mitigate by checking structural readability/visibility and using screenshots for human visual QA.

## Implementation Boundaries

The implementation should stay inside:

- `frontend/src/app`
- `frontend/src/ui`
- `frontend/src/styles`
- `frontend/src/features/tasks`
- `frontend/src/features/reviews`
- `frontend/src/features/agents`
- `frontend/e2e`
- frontend package manifest and lockfile if `lucide-react` is added

No backend changes are expected.

## Spec Patch

No further OpenSpec spec patch is required. The accepted review refinements have already been applied to `openspec/changes/improve-console-ui-ux/specs/console-ui-ux/spec.md`.
