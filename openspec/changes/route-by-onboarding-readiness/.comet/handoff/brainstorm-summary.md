# Brainstorm Summary

- Change: route-by-onboarding-readiness
- Date: 2026-07-11

## 确认的需求事实

- Agents 保留为管理模块，但不再作为默认入口。
- GitHub App 已配置且已安装、并至少接入一个仓库时进入 `/tasks`；否则进入 `/onboarding`。
- `GITHUB_APP_PUBLIC_BASE_URL` 在 Web 模式下必填，不使用请求 Host 或转发头推导。
- 不新增 API 或 onboarding 持久化状态。

## 确认的技术方案

- 新增纯判定函数与入口路由组件：函数根据 repository onboarding summary 返回 `/tasks` 或 `/onboarding`，组件负责加载、错误降级和 replace navigation。
- `/` 与通配路由渲染入口组件；本地登录成功跳转 `/`，显式业务路由保持不变。
- 后端在 `config.Load` 中集中解析并规范化公网 origin；manifest 服务继续只消费已经校验的配置值。
- URL 仅允许 `http/https`、非空 host、无 userinfo/query/fragment，path 只能为空或 `/`。

## 关键取舍与风险

- 复用现有 summary 可避免新增接口，但接口暂时失败时会保守进入 onboarding。
- 启动时失败可尽早暴露部署错误，但要求所有 Web 部署在升级前补齐环境变量。
- 不信任 Host/forwarded headers 缩小攻击面，但反向代理不能替代显式公网配置。

## 测试策略

- Go 配置测试采用表驱动覆盖缺失、相对 URL、非法 scheme、userinfo、query、fragment、子路径、合法值及末尾斜杠规范化。
- Manifest 单元测试解析生成 JSON，断言 callback/setup URL 完全来自配置。
- React 路由测试覆盖 ready、App 未安装、无仓库、请求失败、登录后进入统一入口，以及显式 `/agents` 不被改写。
- 最终运行相关单测、后端/前端构建及项目验证命令。

## Spec Patch

无。现有 delta specs 已覆盖核心成功、未就绪、请求失败、非法配置和请求头不影响配置等边界场景。
