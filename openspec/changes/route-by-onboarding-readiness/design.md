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
