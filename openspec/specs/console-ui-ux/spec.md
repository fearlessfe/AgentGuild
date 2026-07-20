# console-ui-ux Specification

## Purpose
Define the console's responsive UI/UX baseline: readable review diffs, mobile workbench lists and controls, consistent iconography, loading/mutation feedback, and onboarding-readiness-aware navigation.
## Requirements
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

### Requirement: Repository onboarding is discoverable
The console SHALL make repository onboarding discoverable from persistent authenticated navigation or an equivalently prominent settings entry.

#### Scenario: User scans authenticated navigation
- **WHEN** an authenticated user views the application shell
- **THEN** the repository onboarding entry is visible without first opening the overview/dashboard page

#### Scenario: User identifies current module
- **WHEN** an authenticated user opens the repository onboarding route
- **THEN** the top-level module label and page header identify the repository onboarding context

### Requirement: Repository onboarding separates setup from automation
The console SHALL present repository access setup separately from Issue sync rule automation.

#### Scenario: User opens sync rules
- **WHEN** a user opens the sync rule page
- **THEN** the page focuses on automation rules
- **AND** repository access setup is linked to the repository onboarding module rather than embedded as the primary workflow

#### Scenario: User opens repository onboarding
- **WHEN** a user opens the repository onboarding module
- **THEN** adding or selecting a repository does not automatically create an Issue sync rule

### Requirement: Default console entry follows onboarding readiness
控制台 SHALL 使用服务端仓库接入汇总状态决定隐式入口。仅当租户的 GitHub App 已配置并安装，且至少存在一个已接入仓库时，控制台 MUST 将用户送入任务中心；否则 MUST 将用户送入首次引导。

#### Scenario: Ready tenant enters task center
- **WHEN** 已登录用户进入根路径、完成本地登录或访问未知控制台路径
- **AND** GitHub App 状态为已配置且 installation ID 大于零
- **AND** 已接入仓库列表至少包含一项
- **THEN** 控制台使用 replace navigation 进入 `/tasks`

#### Scenario: Tenant without complete onboarding enters onboarding
- **WHEN** 已登录用户进入隐式控制台入口
- **AND** GitHub App 未配置、尚未安装或没有已接入仓库中的任一条件成立
- **THEN** 控制台使用 replace navigation 进入 `/onboarding`

#### Scenario: Readiness cannot be loaded
- **WHEN** 控制台无法取得仓库接入汇总状态
- **THEN** 控制台进入 `/onboarding`
- **AND** 不得错误进入 `/tasks`

#### Scenario: User explicitly opens a management route
- **WHEN** 已登录用户显式访问 `/agents`、`/tasks` 或 `/onboarding`
- **THEN** 控制台保留该显式路由
- **AND** Agents 仍作为管理模块可用

