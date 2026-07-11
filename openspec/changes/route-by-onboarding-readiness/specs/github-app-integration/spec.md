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
