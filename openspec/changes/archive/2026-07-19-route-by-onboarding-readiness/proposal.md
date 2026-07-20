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
