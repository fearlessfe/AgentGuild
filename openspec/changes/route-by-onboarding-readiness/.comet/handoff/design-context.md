# Comet Design Handoff

- Change: route-by-onboarding-readiness
- Phase: design
- Mode: compact
- Context hash: ffb3dedf75d85767928251c50991c45fd3915f054edb253fb5df695b79decded

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/route-by-onboarding-readiness/proposal.md

- Source: openspec/changes/route-by-onboarding-readiness/proposal.md
- Lines: 1-29
- SHA256: 4aca587765e30d1bab6039346adffd983672de0cbc698aec8dc69894c7a144a7

```md
## Why

控制台当前在本地登录成功后固定进入 Agents 页面，而未知入口进入任务中心，导致默认落点不一致，也无法在 GitHub App 或仓库尚未接入时引导管理员完成必要配置。同时，GitHub App manifest 允许在公网基础 URL 为空时生成相对回调地址，最终被 GitHub 拒绝。

## What Changes

- 控制台统一通过接入就绪状态决定默认入口：GitHub App 已创建并安装、且至少存在一个已接入仓库时进入 `/tasks`，否则进入 `/onboarding`。
- `/`、本地登录成功和未知前端路由统一使用同一入口判定，不再默认进入 Agents 页面。
- 复用现有 `GET /v1/repository-onboarding` 汇总数据，不新增 onboarding 状态接口或持久化状态。
- 在 `WEB_ENABLED=true` 时将 `GITHUB_APP_PUBLIC_BASE_URL` 设为必填配置，并校验为无 query/fragment 的绝对 HTTP(S) URL。
- GitHub App manifest 仅使用显式配置的公网基础 URL，不从 `Host`、`X-Forwarded-Host` 或其他请求头推导。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `console-ui-ux`: 默认入口根据租户的 GitHub App 安装状态和仓库接入状态进行路由。
- `github-app-integration`: Web 模式下要求合法的公网基础 URL，并仅使用该配置生成 manifest 回调地址。

## Impact

- 前端：应用路由、登录成功跳转、入口加载/失败状态及对应组件测试。
- 后端：配置加载与校验、GitHub App manifest 配置约束及对应单元测试。
- 部署：所有启用 Web 的环境必须设置 `GITHUB_APP_PUBLIC_BASE_URL`；未配置或格式非法时进程启动失败。
- API：不新增或修改 REST/MCP 接口。
```

## openspec/changes/route-by-onboarding-readiness/design.md

- Source: openspec/changes/route-by-onboarding-readiness/design.md
- Lines: 1-74
- SHA256: 1bf27d97fa6291b7c2af003a7fa8bd7169aba7b44546bec3308b822fa2bbf0d8

```md
## Context

控制台已有 `GET /v1/repository-onboarding`，一次返回租户的 GitHub App 公共状态、App 可见仓库和已接入仓库。当前 `/login` 成功后硬编码跳转 `/agents`，通配路由则跳转 `/tasks`，而 `/onboarding` 仍声明服务端没有 onboarding 状态。后端 manifest 服务直接拼接可选的 `GITHUB_APP_PUBLIC_BASE_URL`，空值会生成 GitHub 不接受的相对 URL。

本变更跨越前端入口路由与后端启动配置，但不改变 API、数据模型或权限模型。接入就绪状态必须由已有服务端事实数据推导，不能持久化为容易失真的前端状态。

## Goals / Non-Goals

**Goals:**

- 统一 `/`、登录成功和未知路由的默认入口行为。
- 在 GitHub App 已创建并完成安装、且至少有一个已接入仓库时进入任务中心。
- 在任一接入条件不满足或状态请求失败时进入 onboarding。
- 阻止 Web 服务使用缺失或非法的公网基础 URL 启动。
- 保证 manifest 回调地址只由显式配置产生，不信任请求 Host 或转发头。

**Non-Goals:**

- 不新增 onboarding 状态表或 API。
- 不要求完成同步规则或成功同步任务后才视为就绪。
- 不移除 Agents 页面或导航入口。
- 不修改 GitHub App、仓库接入接口的响应结构。
- 不从 `Host`、`X-Forwarded-Host` 或 `X-Forwarded-Proto` 推导公网地址。

## Decisions

### 1. 使用 repository onboarding summary 作为唯一入口判定数据源

前端入口组件调用现有 `getRepositoryOnboarding()`。仅当以下条件全部成立时返回 `/tasks`：

- `github_app.configured === true`
- `github_app.installation_id` 为大于零的数值
- `onboarded_repositories.items` 至少包含一项

其他成功响应均进入 `/onboarding`。请求失败也进入 `/onboarding`，避免在无法确认租户已就绪时错误放行。

备选方案是新增 `/v1/onboarding/status`，但当前只会重复聚合已有 summary，增加接口维护成本。分别请求 GitHub App 与仓库接口会引入额外请求和状态竞态，也不采用。

### 2. 所有隐式入口统一经过一个前端组件

新增无页面装饰的入口判定组件，在加载时呈现可访问的轻量状态，取得结果后使用 replace navigation。`/` 和通配路由渲染该组件；本地登录成功跳转 `/`，从而复用同一逻辑。显式访问 `/agents`、`/tasks` 或 `/onboarding` 不被强制重定向，管理员仍可按需管理 Agents 或回看引导页。

把入口判定放在路由层而不是登录组件中，可以覆盖已有 session 直接打开根路径和未知路径的场景。

### 3. Web 模式启动时严格校验公网基础 URL

配置加载在 `WEB_ENABLED=true` 时要求 `GITHUB_APP_PUBLIC_BASE_URL` 非空。使用 `net/url` 解析并要求：

- scheme 只能是 `http` 或 `https`
- host 非空
- userinfo、query、fragment 均为空
- path 为空或仅为 `/`

合法值规范化为去除末尾 `/` 的 origin。限制为 origin 可避免将固定回调路径错误拼接到已有子路径下。生产环境应使用 HTTPS；保留 HTTP 是为了现有本地开发环境。

备选方案是从请求 Host 或转发头推导，但这会扩大 Host header poisoning 和代理信任边界，因此不采用。另一个备选是只在 manifest 请求时返回错误，但启动失败能更早暴露部署错误。

## Risks / Trade-offs

- **启用 Web 的现有部署未设置新必填值将无法启动** → 更新部署文档和示例环境，发布前显式配置公网 origin。
- **summary 接口暂时失败会把已完成接入的用户送到 onboarding** → onboarding 保留通往 Git 接入和仓库接入的恢复路径；不将失败缓存为永久状态。
- **完成标准暂不包含同步规则** → 本次严格采用已确认的 GitHub App + 仓库标准；后续如业务定义变化，再扩展服务端聚合接口。
- **HTTP URL 在公网部署不安全** → 文档明确生产使用 HTTPS；保留 HTTP 仅服务本地测试与开发。

## Migration Plan

1. 在所有 `WEB_ENABLED=true` 环境配置 `GITHUB_APP_PUBLIC_BASE_URL` 为平台公网 origin。
2. 部署后端配置校验，再部署前端入口分流。
3. 验证未配置租户进入 `/onboarding`，完成 GitHub App 安装并接入仓库的租户进入 `/tasks`。
4. 回滚时同时回滚代码；环境变量可保留，不影响旧版本。

## Open Questions

无。
```

## openspec/changes/route-by-onboarding-readiness/tasks.md

- Source: openspec/changes/route-by-onboarding-readiness/tasks.md
- Lines: 1-13
- SHA256: 9cc33b86be83ae518e0d882e032ca7ef0c06f93f9cace5d6e7985c4b2e2a3bd1

```md
## 1. 后端公网 URL 配置

- [ ] 1.1 先补充配置单元测试，覆盖 Web 模式缺失、非法及合法 `GITHUB_APP_PUBLIC_BASE_URL`，确认测试按预期失败后实现严格解析与规范化。
- [ ] 1.2 补充 manifest 测试，证明 callback/setup URL 只使用配置值且不受请求头影响，并同步更新部署文档中的必填变量说明。

## 2. 前端默认入口

- [ ] 2.1 先补充入口路由测试，覆盖已就绪、未安装、无已接入仓库和请求失败场景，确认测试按预期失败。
- [ ] 2.2 实现共享入口判定组件，将根路径、登录成功和未知路由统一接入判定，同时保留 Agents 等显式路由。

## 3. 验证

- [ ] 3.1 运行后端相关测试与构建、前端单元测试与构建，并核对 OpenSpec 验收场景与实现一致。
```

## openspec/changes/route-by-onboarding-readiness/specs/console-ui-ux/spec.md

- Source: openspec/changes/route-by-onboarding-readiness/specs/console-ui-ux/spec.md
- Lines: 1-25
- SHA256: 80b18ab12115f6bef285007b3e89495d9ba7f8ac4f4a5160b4a21e5727918213

```md
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
```

## openspec/changes/route-by-onboarding-readiness/specs/github-app-integration/spec.md

- Source: openspec/changes/route-by-onboarding-readiness/specs/github-app-integration/spec.md
- Lines: 1-21
- SHA256: 8ca1e468b14158a652958c1a2055f162d33b3f91884d186e8e97f8f66e0e139d

```md
## ADDED Requirements

### Requirement: GitHub App manifest uses an explicit public origin
启用 Web transport 时，系统 MUST 要求配置 `GITHUB_APP_PUBLIC_BASE_URL`，并且该值 MUST 是不含 userinfo、query、fragment 或子路径的绝对 HTTP(S) origin。系统 SHALL 仅使用该配置生成 GitHub App manifest 的 callback 与 setup URL，不得从请求 Host 或转发头推导公网地址。

#### Scenario: Web service starts with a valid public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 是合法 HTTP(S) origin
- **THEN** 配置加载成功
- **AND** manifest callback 与 setup URL 使用规范化后的 origin

#### Scenario: Web service starts without a public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 为空
- **THEN** 配置加载失败并明确指出缺少该变量

#### Scenario: Web service starts with an invalid public origin
- **WHEN** `WEB_ENABLED=true` 且 `GITHUB_APP_PUBLIC_BASE_URL` 是相对地址、缺少 host、使用非 HTTP(S) scheme，或包含 userinfo、query、fragment、子路径
- **THEN** 配置加载失败并明确指出该变量无效

#### Scenario: Request headers disagree with configured origin
- **WHEN** manifest 请求的 `Host` 或任一 `X-Forwarded-*` 请求头与 `GITHUB_APP_PUBLIC_BASE_URL` 不同
- **THEN** 生成的 callback 与 setup URL 仍只使用 `GITHUB_APP_PUBLIC_BASE_URL`
```

