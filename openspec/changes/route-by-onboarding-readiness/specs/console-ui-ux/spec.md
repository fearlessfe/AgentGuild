## ADDED Requirements

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
