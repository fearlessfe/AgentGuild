## 1. Verification Baseline

- [x] 1.1 Add or update Playwright helpers to measure mobile page-level horizontal overflow and key control visibility.
- [x] 1.2 Add a regression check that captures the current unreadable diff wrapping on desktop and 375px mobile viewports, then make it pass with the implementation.
- [x] 1.3 Add mobile checks for task and agent list core information visibility.

## 2. Review Diff Readability

- [x] 2.1 Update review diff CSS so code line content preserves readable text and avoids character-level wrapping.
- [x] 2.2 Add explicit diff-region overflow behavior that does not create page-level horizontal scrolling.
- [x] 2.3 Improve mobile review diff behavior by defaulting to the most readable mode or presenting split mode inside an obvious scroll container.
- [x] 2.4 Verify line comments and diff mode toggles remain usable after the layout change.

## 3. Mobile Task And Agent Lists

- [x] 3.1 Adapt the task list for mobile so each item exposes id, title, status, and repository or publisher context without silent clipping.
- [x] 3.2 Adapt the agent list for mobile so each item exposes name, status, owner or team context, and navigation affordance without silent clipping.
- [x] 3.3 Preserve dense table behavior and existing filtering behavior on desktop.

## 4. Icon And Interaction Baseline

- [x] 4.1 Add the selected SVG icon dependency if needed and replace structural Unicode icons in the rail, topbar, empty states, and provider cards.
- [x] 4.2 Ensure icon-only interactive controls have accessible names and inherit semantic theme colors.
- [x] 4.3 Apply mobile hit-target improvements for rail items, icon buttons, tabs, selects, and primary actions while preserving desktop density.
- [x] 4.4 Confirm focus-visible states remain clear in dark and light themes.

## 5. Feedback States

- [ ] 5.1 Improve loading states for task, agent, and review regions that currently render plain text or sparse placeholders.
- [ ] 5.2 Ensure review decision actions and comment submission controls communicate pending state and prevent duplicate submission.
- [ ] 5.3 Review empty and error states for the affected routes and align them with the shared UI primitives.

## 6. Final Verification

- [ ] 6.1 Run frontend build and unit tests.
- [ ] 6.2 Run targeted Playwright checks for task, agent, and review routes on desktop and mobile viewports.
- [ ] 6.3 Capture or inspect screenshots for the changed core routes and adjust layout issues found during visual QA.
