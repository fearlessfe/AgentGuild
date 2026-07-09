import { Bot, CircleDot, GitBranch, GitPullRequest, LayoutDashboard, Settings, SquareKanban } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { NavLink } from "react-router-dom";

/* Left navigation rail. Each item maps a module to its primary route; icons are
   decorative and the accessible name comes from the aria-label. The rail
   collapses to an icon-only strip or expands to show labels; the caller owns
   that state so the surrounding grid can resize in step. */

type RailItem = { to: string; icon: LucideIcon; label: string };

const RAIL_ITEMS: readonly RailItem[] = [
  { to: "/onboarding", icon: LayoutDashboard, label: "总览" },
  { to: "/sync", icon: GitBranch, label: "同步" },
  { to: "/tasks", icon: SquareKanban, label: "任务中心" },
  { to: "/reviews", icon: GitPullRequest, label: "审核" },
  { to: "/outcome", icon: CircleDot, label: "结果" },
];

type RailProps = { collapsed: boolean; onToggle: () => void };

export function Rail({ collapsed, onToggle }: RailProps) {
  return (
    <nav className="rail" data-collapsed={collapsed} aria-label="主导航">
      <div className="rail-head">
        <span className="rail-brand" aria-hidden="true">
          AG
        </span>
        <button
          type="button"
          className="rail-toggle"
          onClick={onToggle}
          aria-label={collapsed ? "展开侧边栏" : "收起侧边栏"}
          aria-pressed={!collapsed}
        >
          <span aria-hidden="true">{collapsed ? "»" : "«"}</span>
        </button>
      </div>
      {RAIL_ITEMS.map((item) => {
        const Icon = item.icon;
        return (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) => (isActive ? "rail-item active" : "rail-item")}
            aria-label={item.label}
          >
            <span className="rail-icon" aria-hidden="true">
              <Icon size={17} strokeWidth={1.8} />
            </span>
            <span className="rail-label">{item.label}</span>
          </NavLink>
        );
      })}
      <span className="rail-spacer" />
      <NavLink
        to="/agents"
        className={({ isActive }) => (isActive ? "rail-item active" : "rail-item")}
        aria-label="Agents"
      >
        <span className="rail-icon" aria-hidden="true">
          <Bot size={17} strokeWidth={1.8} />
        </span>
        <span className="rail-label">Agents</span>
      </NavLink>
      <span className="rail-item rail-item--static" aria-hidden="true">
        <span className="rail-icon">
          <Settings size={17} strokeWidth={1.8} />
        </span>
        <span className="rail-label">设置</span>
      </span>
    </nav>
  );
}
