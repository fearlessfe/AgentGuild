import { PageHeader, Card, ButtonLink, Steps } from "../../ui";
import type { Step } from "../../ui";

const ONBOARDING_STEPS: Step[] = [
  { title: "登录", state: "done" },
  { title: "Git 接入", state: "current", desc: "连接企业 Git 提供商并授予最小权限" },
  { title: "仓库接入", state: "pending", desc: "选择 App 授权仓库或添加公开仓库" },
  { title: "首次生成", state: "pending", desc: "创建任务生成策略并至少成功运行一次" },
  { title: "任务执行", state: "pending" },
];

export function OnboardingScreen() {
  return (
    <div className="stack">
      <PageHeader title="首次引导" sub="完成仓库接入后即可配置任务生成。首次运行成功后进入任务治理。" />
      <div className="split-2">
        <div className="col">
          <Card title="接入 Git 提供商" sub="当前步骤 · Git 接入">
            <div className="stack">
              <p className="muted text-sm">
                选择企业 Git 提供商并安装 AgentGuild App，仅授予任务治理所需的最小权限。
              </p>
              <div className="row">
                <ButtonLink to="/git-integration" variant="primary">
                  连接 GitHub
                </ButtonLink>
                <ButtonLink to="/repositories">设置仓库</ButtonLink>
                <ButtonLink to="/generation">配置任务生成</ButtonLink>
              </div>
            </div>
          </Card>
        </div>
        <div className="col">
          <Card title="接入路线" sub="按顺序完成 Git、仓库与首次任务生成。">
            <Steps items={ONBOARDING_STEPS} />
          </Card>
        </div>
      </div>
    </div>
  );
}
