import { PageHeader, Card, Button, StatusChip, DenseTable, ApiNote } from "../../ui";
import type { DenseRow } from "../../ui";

const REPO_ROWS: DenseRow[] = [
  {
    cells: ["billing-service", <StatusChip tone="success">已启用</StatusChip>, <code>main</code>, "private"],
    selected: true,
  },
  { cells: ["event-gateway", <StatusChip tone="success">已启用</StatusChip>, <code>main</code>, "private"] },
  { cells: ["legacy-monolith", <StatusChip tone="neutral">已停用</StatusChip>, <code>master</code>, "private"] },
];

const RULE_FIELDS: [string, string][] = [
  ["包含标签", "agent-ready"],
  ["排除标签", "security-hold, needs-product"],
  ["Issue 状态", "open"],
  ["任务类型", "code"],
  ["默认优先级", "P1"],
  ["同步频率", "每 5 分钟"],
  ["重复策略", "更新现有 Task"],
];

export function SyncRuleScreen() {
  return (
    <div className="stack">
      <PageHeader
        title="仓库与同步规则"
        sub="选择要纳入治理的仓库，并用规则驱动 Issue → Task 同步。"
        actions={
          <>
            <Button>保存草稿</Button>
            <Button icon="⤿">预览</Button>
            <Button variant="primary">启用规则</Button>
          </>
        }
      />
      <div className="split-2">
        <div className="col col--fill">
          <Card title="安装仓库" sub="3 个仓库" pad={false}>
            <DenseTable
              columns={["仓库", "同步", "默认分支", "可见性"]}
              rows={REPO_ROWS}
              caption="安装仓库列表"
            />
          </Card>
        </div>
        <div className="col">
          <Card title="同步规则">
            <div className="rule-grid">
              {RULE_FIELDS.map(([label, value]) => (
                <div className="rule-field" key={label}>
                  <span className="field-label">{label}</span>
                  <div className="input">{value}</div>
                </div>
              ))}
            </div>
          </Card>
          <Card title="自然语言预览">
            <p className="muted text-sm">
              每 5 分钟同步 <strong>billing-service</strong> 中带 <code>agent-ready</code>、状态为 open 的 Issue，排除{" "}
              <code>security-hold</code> 与 <code>needs-product</code>，生成 P1 的 code 任务；已存在的 Task 将被更新。
            </p>
          </Card>
          <div className="row-between">
            <Button variant="ghost">暂停规则</Button>
            <span className="faint text-xs">影响：预计新增 8 个任务</span>
          </div>
          <ApiNote status="planned">仓库目录与同步规则 CRUD/预览/启停均为规划能力。</ApiNote>
        </div>
      </div>
    </div>
  );
}
