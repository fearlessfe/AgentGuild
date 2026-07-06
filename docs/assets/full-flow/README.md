# AgentGuild 全流程设计交付

## 可编辑源文件

代码原稿位于：

`design/agentguild-full-flow/`

入口文件为 `design/agentguild-full-flow/index.html`。该目录使用原生 HTML、CSS 与 JavaScript，不依赖产品 React 应用。

## 路由格式

```text
index.html?screen=<screen-id>&theme=<dark|light>
```

可用的 `screen-id`：

`journey`、`api-coverage`、`login`、`onboarding`、`git-integration`、`repository-sync-rule`、`sync-result`、`task-center`、`execution-detail`、`submission-validation`、`review-workspace`、`outcome`、`components-states`、`responsive-rules`、`gallery`。

未传 `screen` 时打开 `gallery`，未传或传入无效 `theme` 时使用 `dark`。无效 `screen` 会显示错误与全部有效 ID。

## API 标注图例

- `API · available`：当前接口可直接支撑。
- `API · partial`：当前接口只能支撑部分信息，或 REST 已有但前端尚未接入。
- `API · planned`：需要新增后端能力。
- `API · auth fix`：接口存在，但认证或授权需先修正。

这些 API 标注、规格说明和标尺只属于设计交付注释，不属于最终用户界面。

## 导出

运行以下命令，使用仓库内 `frontend/` 的 Playwright Chromium 在 1440×1024（2x）下重新生成全部 PNG 与多页 PDF：

```bash
node design/render-mockups.mjs
```

脚本会启动一个临时静态服务器（ES 模块导入需经 `http://`），逐个渲染 `screen`/`theme` 路由，并以 `?print=all` 打印生成 14 页 PDF。

## 导出清单

- `00-full-flow-journey.png`
- `00-api-coverage.png`
- `01-login-{dark,light}.png`
- `02-onboarding-{dark,light}.png`
- `03-git-integration-{dark,light}.png`
- `04-repository-sync-rule-{dark,light}.png`
- `05-sync-result-{dark,light}.png`
- `06-task-center-{dark,light}.png`
- `07-execution-detail-{dark,light}.png`
- `08-submission-validation-{dark,light}.png`
- `09-review-workspace-{dark,light}.png`
- `10-outcome-{dark,light}.png`
- `11-components-and-states.png`
- `12-responsive-rules.png`
- `agentguild-full-flow.pdf`

共 20 张画面 PNG、4 张说明板 PNG、1 份 14 页 PDF。API 标注仅出现在说明板，不出现在用户界面画面中。
