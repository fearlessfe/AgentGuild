import { PageHeader, Card, Button, ProviderCard, StatusChip, ApiNote } from "../../ui";

const GITHUB_PERMISSIONS = ["Contents — 只读", "Issues — 读写", "Checks — 只读", "Metadata — 只读"];

export function GitIntegrationScreen() {
  return (
    <div className="stack">
      <PageHeader title="Git 接入" sub="使用通用 Git 提供商外壳；当前仅启用 GitHub。" />
      <div className="split-2">
        <div className="col">
          <ProviderCard logo="◐" name="GitHub" note="已连接 · Atlas Billing 组织" permissions={GITHUB_PERMISSIONS}>
            <div className="stack-sm">
              <div className="row-between">
                <span className="card-sub">状态</span>
                <StatusChip tone="success">已配置</StatusChip>
              </div>
              <div className="field">
                <label className="field-label">GitHub App 私钥</label>
                <div className="input">
                  <span className="faint">已保存 · 不可读取</span>
                </div>
                <p className="field-hint">出于安全，已存储的私钥永不回显。</p>
              </div>
              <div className="row">
                <Button icon="⤿">检测连接</Button>
                <Button>更新配置</Button>
                <Button variant="danger">删除</Button>
              </div>
            </div>
          </ProviderCard>
        </div>
        <div className="col">
          <ProviderCard logo="◑" name="GitLab" note="即将支持" disabled>
            <div className="row-between">
              <span className="card-sub">状态</span>
              <StatusChip tone="neutral">即将支持</StatusChip>
            </div>
          </ProviderCard>
          <ApiNote status="available">GET/POST/DELETE /v1/github-app 已提供 CRUD；前端仍待接入。</ApiNote>
          <ApiNote status="planned">连接检测（connection test）动作需要新增后端能力。</ApiNote>
        </div>
      </div>
    </div>
  );
}
