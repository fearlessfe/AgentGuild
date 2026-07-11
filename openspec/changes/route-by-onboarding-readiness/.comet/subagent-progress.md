# Subagent Progress

- Change: `route-by-onboarding-readiness`
- Review mode: `standard`
- TDD mode: `tdd`

## Current task

- Plan task: `Task 3: 实现统一的前端 onboarding 就绪入口`
- OpenSpec task: `2.1 先补充入口路由测试，覆盖已就绪、未安装、无已接入仓库和请求失败场景，确认测试按预期失败；2.2 实现共享入口判定组件，将根路径、登录成功和未知路由统一接入判定，同时保留 Agents 等显式路由。`
- Stage: `done`
- Review/fix round: `0/1`
- Commit: `d0a097f`
- Changed files: `frontend/src/app/HomeRedirect.tsx`, `frontend/src/app/HomeRedirect.test.tsx`, `frontend/src/app/AppShell.tsx`, `frontend/src/features/auth/LoginPage.tsx`, `frontend/src/features/auth/LoginPage.test.tsx`
- RED evidence: `npm test -- --run src/app/HomeRedirect.test.tsx` failed because `HomeRedirect` did not exist.
- GREEN evidence: `npm test -- --run src/app/HomeRedirect.test.tsx src/features/auth/LoginPage.test.tsx` passed 12 tests; `npm run build` passed.
- Final review: pending
