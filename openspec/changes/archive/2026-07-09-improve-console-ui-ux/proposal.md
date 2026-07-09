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
