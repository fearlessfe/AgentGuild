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

- [Risk] Adding an icon dependency increases frontend bundle size. → Mitigation: use a tree-shakeable icon package and import only used icons.
- [Risk] Mobile cards could diverge from desktop table semantics. → Mitigation: share the same source data and keep desktop table behavior unchanged.
- [Risk] Horizontal diff scrolling can be less convenient on small screens. → Mitigation: default mobile to the most readable mode and keep mode controls available.
- [Risk] Changing CSS primitives may affect legacy pages. → Mitigation: scope mobile target changes to breakpoints and verify affected core routes.

## Migration Plan

1. Update frontend components and styles in place.
2. Add any required icon dependency to the frontend package manifest and lockfile.
3. Extend Playwright/Vitest coverage for the targeted UX baseline.
4. Run `cd frontend && npm run build`, `npm test -- --run`, and relevant Playwright specs.

Rollback is straightforward: revert the frontend commit; no persistent data migration is involved.

## Resolved UX Choices

- Mobile review defaults to Unified mode; mode controls remain available for reviewers who need Split.
- Task and agent mobile lists use compact cards as the primary responsive presentation; table scroll wrappers are reserved for secondary dense data if needed later.
