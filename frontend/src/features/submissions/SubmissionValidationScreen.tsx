import { PageHeader, Card, Button, CheckRow, ApiNote } from "../../ui";
import type { CheckState } from "../../ui";
import { sharedData } from "../shared/mockData";

type Check = { name: string; state: CheckState; meta?: string; value?: string };

const CHECKS: Check[] = [
  { name: "Commit 存在", state: "pass", meta: `${sharedData.commit} 可达` },
  { name: "分支匹配", state: "pass", meta: sharedData.branch },
  { name: "Base 为祖先", state: "pass", meta: `${sharedData.baseCommit} 是祖先` },
  { name: "构建", state: "pass" },
  { name: "公共测试", state: "pass", value: "42/42" },
  { name: "隐藏测试", state: "warn", value: "18/20" },
  { name: "安全扫描", state: "pass" },
  { name: "变更范围", state: "warn", meta: "触及结算核心路径" },
];

export function SubmissionValidationScreen() {
  return (
    <div className="stack">
      <PageHeader
        title="提交与验证"
        sub={`${sharedData.taskId} · 验证 attempt 1`}
        actions={<Button icon="⤿">平台重试</Button>}
      />
      <div className="split-2">
        <div className="col">
          <Card title="提交信息">
            <div className="fact-grid">
              <div className="fact">
                <span className="ctx-label">repo</span>
                <code>{sharedData.repository}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">branch</span>
                <code>{sharedData.branch}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">base</span>
                <code>{sharedData.baseCommit}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">commit</span>
                <code>{sharedData.commit}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">attempt</span>
                <span>1</span>
              </div>
            </div>
          </Card>
          <ApiNote status="partial">
            GET /v1/submissions/{"{id}"} 与 /diff 已有；提交详情页仍需补齐。
          </ApiNote>
          <ApiNote status="planned">验证 job 明细、attempt、lease 与日志需要新增只读查询。</ApiNote>
        </div>
        <div className="col col--fill">
          <Card title="验证检查" sub="8 项" pad={false}>
            {CHECKS.map((check) => (
              <CheckRow key={check.name} name={check.name} state={check.state} meta={check.meta} value={check.value} />
            ))}
          </Card>
          <p className="field-hint">仅平台错误提供“平台重试”；代码检查失败不提供重试。</p>
        </div>
      </div>
    </div>
  );
}
