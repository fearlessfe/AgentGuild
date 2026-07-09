# Brainstorm Summary

- Change: repository-onboarding-center
- Date: 2026-07-09

## 已确认事实

- 用户希望优先做好“仓库接入中心”，而不是继续扩展 Issue 同步规则创建流程。
- 模块需要支持两类仓库来源：GitHub App installation 下的仓库，以及公开 GitHub 仓库。
- 当前体验问题是入口只能从总览按钮进入，缺少查看 GitHub App 和公开仓库信息的独立位置。
- 用户希望流程分为两步：安装/查看 GitHub App 是单独一步；添加/查看仓库是另外一步。
- 添加仓库时既可以从已安装 GitHub App 可访问的仓库中选择，也可以手动填入开源仓库地址。
- 模块还需要让管理员方便查看已安装的 App 和已经添加的仓库。
- 现有前端有 `/git-integration`、`/sync`、`Rail`；`/sync` 当前混合了 App 仓库列表、公开仓库输入和同步规则。
- 现有后端有 GitHub App 配置表与 App installation repository listing；公开仓库目前主要通过 public issue source 服务同步规则使用，缺少独立 repository inventory。

## 候选技术方案

### 方案 A：最小持久化 inventory（推荐候选）

新增 tenant-scoped `repositories` 或 `repository_sources` 表，只持久化平台纳入管理的仓库：App 仓库选择状态和公开仓库记录。App installation repository listing 仍实时来自 GitHub App，UI 将“可用”与“已纳入”分开展示。

### 方案 B：纯前端/复用同步规则

不新增 inventory，把 App 仓库列表和 public sync rule 继续复用到新页面。实现快，但公开仓库仍等同同步规则，无法满足“先接入仓库，不自动创建规则”的目标。

### 方案 C：完整仓库源聚合服务

新增统一 repository source service，聚合 App discovery、public repository metadata、未来 provider 扩展和 sync rule 引用。边界最清晰，但对当前目标偏重。

## 待确认问题

- 无。用户已确认采用两步流程：安装/查看 GitHub App，然后添加/查看仓库。

## 当前推荐

推荐方案 A 的数据边界不变：新增轻量 repository inventory，保持 App discovery 实时，持久化用户选择和公开仓库。交互上推荐“两步但同一模块”：Step 1 管理 GitHub App 连接，Step 2 添加仓库；页面同时提供“已安装 App”和“已添加仓库”的可扫描列表。

## 确认的交互方向

- 新增独立的“仓库接入”主入口，不能只依赖总览页按钮。
- 仓库接入模块按两步呈现：
  1. 安装/查看 GitHub App。
  2. 添加/查看仓库。
- 添加仓库支持两种路径：
  - 从 GitHub App 可访问仓库中选择并添加。
  - 输入公开 GitHub 仓库地址并添加。
- 已安装 App 和已添加仓库都必须方便查看。

## 候选实现修改

### 前端

- 在 `Rail` 增加“仓库接入”入口，路由建议为 `/repositories`。
- 新增 `RepositoryOnboardingScreen`，包含 GitHub App 步骤、添加仓库步骤、已安装 App 摘要、已添加仓库列表。
- `GitIntegrationScreen` 可以继续存在，但从仓库接入页跳转进入；或后续合并为仓库接入页的 Step 1。
- `SyncRuleScreen` 移除“公共仓库同步=添加仓库”的主流程，把它改为“基于已添加仓库创建同步规则”的后续入口。
- `api/client.ts` 增加 repository onboarding 类型、API helper、demo 数据。

### 后端

- 新增 tenant-scoped onboarded repositories 持久化表，记录来源、仓库全名、默认分支、可见性、创建/更新时间。
- 新增 service/repository 层，提供 list、add public repo、add/select app repo、remove。
- 复用现有 `GET /v1/repositories` 或新增更清晰的 onboarding endpoint 区分“App 可访问仓库”和“已添加仓库”。
- public repo 添加至少校验 `owner/repo` 格式，尽量通过公开 GitHub API 补全默认分支和 visibility。

### 测试

- 后端覆盖 tenant isolation、admin-only mutation、public repo validation、App repo selection。
- 前端覆盖导航入口、GitHub App 状态、从 App 添加仓库、添加公开仓库、已添加仓库列表。

## 测试策略候选

- 后端：migration、repository store、service、REST，覆盖 tenant isolation、admin mutation、public repo validation、App listing failure。
- 前端：Rail 入口、repository onboarding 页面、GitHub App unconfigured/configured/error states、App repo selection、public repo add demo flow。

## Spec Patch

已回写：
- `specs/repository-onboarding-center/spec.md`：补充两步流程要求，以及已安装 App 和已添加仓库可查看场景。
- `specs/github-app-integration/spec.md`：补充已安装 App 公开详情可查看且不暴露私钥的场景。
