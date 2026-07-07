import { PageHeader, Card, Button, Steps, ApiNote } from "../../ui";
import type { Step } from "../../ui";

const ONBOARDING_STEPS: Step[] = [
  { title: "登录", state: "done" },
  { title: "Git 接入", state: "current", desc: "连接企业 Git 提供商并授予最小权限" },
  { title: "仓库", state: "pending" },
  { title: "同步", state: "pending" },
  { title: "完成", state: "pending" },
];

export function OnboardingScreen() {
  return (
    <div className="stack">
      <PageHeader title="首次引导" sub="完成四步接入后即可开始任务同步。成功同步一次才视为引导完成。" />
      <div className="split-2">
        <div className="col">
          <Card title="接入 Git 提供商" sub="当前步骤 · Git 接入">
            <div className="stack">
              <p className="muted text-sm">
                选择企业 Git 提供商并安装 AgentGuild App，仅授予任务治理所需的最小权限。
              </p>
              <div className="row">
                <Button variant="primary">连接 GitHub</Button>
                <Button variant="ghost">稍后再说</Button>
              </div>
            </div>
          </Card>
          <ApiNote status="planned">
            服务端尚无 onboarding 状态接口；完成状态必须由服务端持久化，不能用前端本地状态代替。
          </ApiNote>
        </div>
        <div className="col">
          <Card title="引导进度">
            <Steps items={ONBOARDING_STEPS} />
          </Card>
          <div className="row-between">
            <Button variant="primary">保存并继续</Button>
            <span className="faint text-xs">进度已保存</span>
          </div>
        </div>
      </div>
    </div>
  );
}
