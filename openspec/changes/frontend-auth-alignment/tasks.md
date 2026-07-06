## 1. 需求分析与契约确认

- [x] 1.1 列出所有人类控制台会调用的 REST 接口清单
- [x] 1.2 确认哪些接口当前只接受 Bearer token、哪些已接受 session

## 2. 后端中间件实现

- [x] 2.1 在 `backend/internal/transport/rest/session_middleware.go` 中新增 `authenticateHumanOrAgent` 组合中间件
- [x] 2.2 在 `backend/internal/transport/rest/router.go` 中把任务/执行/审核/声望/提交 diff 路由切换到组合中间件
- [x] 2.3 保留 Agent 自服务接口的纯 Bearer token 鉴权

## 3. 测试与验证

- [x] 3.1 更新/新增 REST 路由测试，验证 session cookie 可访问任务列表
- [x] 3.2 更新/新增 REST 路由测试，验证 Bearer token 仍可访问任务列表
- [x] 3.3 运行 `go test -race ./internal/transport/rest/...`
- [x] 3.4 运行前端单元测试 `npm test -- --run`

## 4. 文档与状态更新

- [x] 4.1 在 change 的 tasks.md 中勾选完成的任务
- [x] 4.2 运行 Comet build 阶段守卫
