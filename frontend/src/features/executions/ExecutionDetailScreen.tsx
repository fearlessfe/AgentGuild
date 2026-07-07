import { PageHeader, Card, StatusChip, Timeline, ApiNote } from "../../ui";
import type { TimelineEvent } from "../../ui";
import { sharedData } from "../shared/mockData";

const EVENTS: TimelineEvent[] = [
  { time: "17:02:10", title: "任务已领取", note: "Atlas v12 获得 lease" },
  { time: "17:02:14", title: "执行已开始" },
  { time: "17:02:15", title: "凭证已签发", note: "短期 Git 令牌 · 平台代理" },
  { time: "17:08:41", title: "运行测试", note: "公共测试 42/42" },
  { time: "17:12:03", title: "Commit 已推送", note: `${sharedData.commit} → ${sharedData.branch}` },
  { time: "17:12:05", title: "等待提交验证" },
];

export function ExecutionDetailScreen() {
  return (
    <div className="stack">
      <PageHeader title="执行详情" sub={`${sharedData.taskId} · 由 ${sharedData.agentVersion} 执行`} />
      <div className="split-2">
        <div className="col">
          <Card title="执行事实">
            <div className="fact-grid">
              <div className="fact">
                <span className="ctx-label">Agent</span>
                <span>{sharedData.agentVersion}</span>
              </div>
              <div className="fact">
                <span className="ctx-label">Lease</span>
                <span>剩余 1h 42m</span>
              </div>
              <div className="fact">
                <span className="ctx-label">最近心跳</span>
                <span>20 秒前</span>
              </div>
              <div className="fact">
                <span className="ctx-label">分支</span>
                <code>{sharedData.branch}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">Base</span>
                <code>{sharedData.baseCommit}</code>
              </div>
              <div className="fact">
                <span className="ctx-label">观测成本</span>
                <span>$0.067</span>
              </div>
              <div className="fact">
                <span className="ctx-label">自报成本</span>
                <span>$0.070</span>
              </div>
              <div className="fact">
                <span className="ctx-label">覆盖率</span>
                <StatusChip tone="warning">partial</StatusChip>
              </div>
            </div>
            <p className="field-hint">不以 progress 百分比表示完成度，改用阶段与事件事实。</p>
          </Card>
          <ApiNote status="partial">
            GET /v1/executions/{"{id}"} 提供 lease/状态/阶段/成本；完整事件与 revision 列表需新增查询。
          </ApiNote>
        </div>
        <div className="col">
          <Card title="事件时间线">
            <Timeline events={EVENTS} />
          </Card>
        </div>
      </div>
    </div>
  );
}
