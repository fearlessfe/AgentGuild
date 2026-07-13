# 多 GitHub App 与仓库接入选择器设计

## 背景

当前系统按租户只保存一个 GitHub App。仓库接入页会通过该 App 读取全部授权仓库，再用一张候选仓库表展示。虽然 GitHub API 客户端已经遍历 `/installation/repositories` 的所有分页，但平台 API 会一次返回全部候选项，前端没有搜索或适合大量仓库的选择交互。

本变更需要同时解决两个问题：

1. 一个租户可以安装和管理多个 GitHub App。
2. 添加仓库时，管理员可以选择一个 App，再从其授权仓库中搜索并选择；也可以不使用 App，直接粘贴公开仓库地址。

## 目标

- 支持租户级多个 GitHub App 连接的新增、安装、查看、检测和删除。
- 使用 GitHub 自动返回的 App 与安装账号元数据标识连接，不引入自定义显示名称。
- 使用统一的“添加仓库”流程，明确区分 GitHub App 仓库与公开仓库。
- GitHub App 仓库列表由后端一次返回全部结果，前端完成不区分大小写的本地搜索。
- 将 GitHub App 来源的已接入仓库绑定到具体 App，确保凭证签发、提交验证和 Issue 同步选择确定的 App。
- 平滑迁移现有单 App 与已接入仓库数据。

## 非目标

- 不为平台仓库列表增加游标或页码分页。
- 不支持自定义 GitHub App 显示名称。
- 不支持同一仓库同时绑定多个 GitHub App。
- 不改变公开 GitHub 仓库的解析规则。
- 不在本次变更中引入 GitLab 等其他 Git Provider。

## 已确认的产品规则

- GitHub App 下拉项显示 GitHub 提供的 `App slug/name · 安装组织或账号`。
- 选择 App 后一次加载该 App 的全部授权仓库，不对平台 API 分页。
- 仓库搜索仅在前端按完整 `owner/repo` 名称执行包含匹配，忽略大小写。
- 同一租户的同一仓库只能接入一次，并且只能绑定一个 GitHub App。
- 如需更换仓库所绑定的 App，管理员必须先移除仓库，再通过另一个 App 重新添加。
- 仍有仓库绑定的 App 不允许删除；管理员必须先移除相关仓库。
- 后端在添加时重新验证仓库属于所选 App，不能信任前端候选列表。

## 数据模型

### GitHub App

`github_apps` 从以 `tenant_id` 为唯一键改为每租户多条记录。新增稳定的本地 `id`，所有管理和关联操作使用 `(tenant_id, id)` 定位，不能仅凭 `id` 绕过租户边界。

每条记录至少包含：

- `id`
- `tenant_id`
- `provider`
- `app_id`
- `installation_id`
- `app_slug`
- `installation_account_login`
- `is_default`
- `private_key`
- `base_url`
- 现有 webhook、client credential 与时间字段

GitHub manifest conversion 创建待安装记录时即生成本地 ID。后续 installation setup 回调必须通过签名状态关联到该记录，不能再用 `tenant_id` 隐式选择唯一 App。安装完成后，服务端从 GitHub 获取 installation 的账号或组织 login 并持久化，用于 UI 展示。

每个租户最多一条记录可标记为 `is_default`。迁移得到的旧 App 自动成为默认 App；此前没有 App 的租户创建第一条 App 时，该记录自动成为默认 App。后续新增 App 不改变默认项。删除无仓库绑定的默认 App 时，在同一事务中将该租户最早创建的剩余 App 设为默认；没有剩余 App 时允许默认项为空。该字段只服务旧单资源 API 的确定性兼容，新前端不依赖它。

### 已接入仓库

`onboarded_repositories` 新增可空的 `github_app_id`：

- `source_type = github_app` 时必须指向同租户的一条 GitHub App。
- `source_type = public_github` 时必须为空。
- 外键使用 `(tenant_id, github_app_id)`，确保跨租户 App 不可关联。
- 仓库保持租户内逻辑唯一，避免同一 `full_name` 同时绑定两个 App 或同时以 App/公开来源重复接入。

删除 GitHub App 使用受限删除语义。存在绑定仓库时返回领域冲突错误，不级联删除仓库。

### 迁移

- 为现有每租户单条 App 生成稳定本地 ID，并保留全部凭证与时间字段。
- 将迁移得到的现有 App 标记为该租户的默认 App，并用约束保证每租户最多一个默认项。
- 对现有 `source_type = github_app` 的仓库，绑定同租户迁移后的 App。
- 若历史脏数据导致无法唯一绑定，迁移应失败并给出可诊断错误，不静默选择。
- 提供完整 up/down 迁移，并在测试数据库迁移列表中注册。

## 后端服务设计

### App 管理

GitHub App repository 与 application service 改为显式的多记录契约：

- 按租户列出 App。
- 按 `(tenant_id, app_id)` 获取、安装、检测和删除单个 App。
- 通过指定 App 创建 GitHub driver 或 IssueSource。
- manifest 和 setup 状态携带待处理的本地 App ID。

连接展示模型不返回私钥、webhook secret、client secret 等敏感字段。

### 仓库候选列表

新增按 App 查询候选仓库的服务方法：

1. 校验 principal 的 tenant。
2. 按 `(tenant_id, github_app_id)` 读取 App。
3. 使用该 App 的 installation credential 调用 GitHub。
4. 沿用现有 GitHub driver 对 GitHub API 分页的完整遍历。
5. 将聚合后的全部仓库一次返回平台前端。

平台端点不增加分页、搜索参数或服务端过滤。

### 添加仓库

添加 GitHub App 仓库的命令包含 `github_app_id` 与 `repo`。应用服务必须：

1. 校验管理员权限与 tenant。
2. 规范化仓库名。
3. 获取指定 App 的全部授权仓库。
4. 精确匹配规范化后的 `full_name`。
5. 检查租户内仓库唯一约束。
6. 保存仓库元数据及 App 绑定。

公开仓库继续通过现有 resolver 校验可访问性，但同样受租户内仓库唯一规则约束。

### 下游 App 解析

当前凭证签发等服务只按 `tenant_id` 获取唯一 driver，必须改为按目标仓库解析：

- Git 凭证签发根据请求中的 repo 查找已接入仓库，再使用其 `github_app_id` 创建 driver。
- 提交 commit 验证沿用执行或仓库信息解析同一绑定，避免与签发阶段使用不同 App。
- Issue 同步规则根据规则的 repo 查找仓库绑定并创建对应 IssueSource。
- 公开仓库路径不得错误使用任意 GitHub App 私钥。

为避免调用方各自实现选择逻辑，应由一个按租户与仓库解析 GitHub App/driver 的应用服务边界统一负责。

## HTTP API

新前端使用以下复数资源 API：

- `GET /v1/github-apps`：列出当前租户的 App。
- `GET /v1/github-apps/{id}`：获取单个 App 的公开信息。
- `DELETE /v1/github-apps/{id}`：删除无仓库绑定的 App。
- `POST /v1/github-apps/{id}:test`：检测单个 App 并返回可见仓库数量。
- `GET /v1/github-apps/{id}/repositories`：一次返回该 App 的全部授权仓库。
- `POST /v1/repositories/github-app`：请求体为 `{ "github_app_id": "...", "repo": "owner/repo" }`。

已有 `/v1/github-app` 单资源路由暂时保留为兼容层，使迁移前的单 App 客户端仍能工作。兼容层只服务迁移后的默认 App；新功能和新前端不依赖该隐式选择。若租户存在多个 App，兼容层也不得随机选择记录。

所有新增读写端点都必须执行 tenant 隔离；变更接口继续遵循现有管理员 session、幂等性和领域错误映射约定。

## 前端设计

### Git 接入页

- 页面顶部提供“新增 GitHub App”。
- 已接入 App 以列表或高密度卡片展示。
- 每项显示 GitHub 自动元数据、安装状态和私钥已安全保存提示。
- 每项提供独立的“继续安装”“检测连接”和“删除”操作。
- 删除被仓库引用的 App 时展示明确提示，引导用户先移除相关仓库。
- 未安装完成的 App 与已安装 App 使用不同状态标识。

### 仓库接入页

移除当前“GitHub App 可见仓库”候选表，将右侧“添加仓库”改为统一表单。

表单使用来源切换：

#### GitHub App 模式

1. App 下拉列出已安装 App，显示 `App · 安装账号/组织`。
2. 选择 App 后加载全部授权仓库。
3. 仓库使用可访问的可搜索 combobox，而不是原生不可搜索的 `select`。
4. 输入搜索词后在浏览器内过滤 `owner/repo`。
5. 已接入的仓库不出现在可选结果中。
6. 选择仓库后展示默认分支与可见性，并启用“添加仓库”。

切换 App 时必须清空仓库选择和搜索词。每个 App 的成功响应在页面生命周期内缓存，切换回来时复用；失败响应不缓存，用户可重试。

#### 公开仓库模式

保持现有 URL 或 `owner/repo` 输入及添加按钮。切换来源时清空另一模式的临时选择，但不影响已接入仓库列表。

### 状态与错误

- 无 App：提示先前往 Git 接入页新增并安装 App。
- App 未安装：不进入仓库候选下拉。
- 仓库加载中：禁用仓库下拉与添加按钮，展示局部 loading。
- 空结果：区分“App 没有授权仓库”和“搜索无匹配”。
- GitHub 请求失败：在 App 仓库选择区域显示错误和重试按钮，不覆盖整页。
- 添加冲突：提示仓库已接入；刷新已接入列表以消除陈旧状态。
- 添加成功：更新已接入列表，清空仓库选择，并从候选项排除该仓库。

## 安全与一致性

- 所有 App、仓库关联与查询均使用复合 tenant 条件。
- App ID 来自请求路径或 body 时必须重新进行 tenant 授权。
- 前端列表仅用于体验，后端必须重新验证 App 对仓库的授权。
- 私钥和 access token 不进入响应、日志、错误信息、测试快照或 demo 数据。
- 删除 App 不级联删除已接入仓库。
- 数据库唯一约束作为并发添加时的最终一致性防线，应用层将唯一冲突映射为稳定领域错误。

## 测试策略

### 后端

- GitHub App repository/application：同租户多 App、跨租户隔离、单项安装/删除/检测。
- Manifest/setup：状态正确关联本地 App ID，两个并行安装流程不会互相覆盖。
- Migration：旧 App 与旧仓库正确绑定；公开仓库保持空绑定。
- Repository onboarding：指定 App 候选列表、授权复验、重复仓库冲突、跨租户 App 拒绝。
- 删除约束：有绑定仓库时拒绝，无绑定时成功。
- Credential、validation、sync：同一租户不同仓库使用各自绑定的 driver/source。
- REST：复数路由契约、权限、错误映射、响应不泄露秘密。
- GitHub driver：继续验证对 GitHub 分页的完整遍历。

### 前端

- Git 接入页渲染多个 App，并把检测、继续安装、删除发送到正确 ID。
- 仓库页来源切换、App 选择、仓库加载和 App 间缓存。
- 搜索忽略大小写并匹配完整 `owner/repo`。
- 已接入仓库过滤、选择详情、添加请求携带正确 App ID。
- 切换 App/来源时清理陈旧选择。
- loading、空候选、搜索无结果、局部错误与重试。
- 公开仓库添加流程保持可用。
- demo mode 与 Playwright 覆盖至少两个 App 和一次可搜索选择添加流程。

## 验收标准

- 一个租户可以完成两个 GitHub App 的独立创建、安装和连接检测。
- Git 接入页能明确区分两个 App 的 GitHub 自动名称和安装账号。
- 仓库接入页可选择任一 App，并从其全部授权仓库中本地搜索后添加。
- 添加请求绑定正确 App；后续 Git 凭证和同步使用同一绑定。
- 已接入仓库无法通过另一 App 重复添加。
- 有仓库绑定的 App 无法删除，并返回可理解的提示。
- 管理员仍可通过粘贴 URL 添加公开仓库。
- 旧单 App 数据升级后继续工作，且既有仓库不会丢失绑定。
