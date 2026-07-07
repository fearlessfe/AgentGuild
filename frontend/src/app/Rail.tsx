import { NavLink } from "react-router-dom";

/* Left navigation rail. Each item maps a module to its primary route; icons are
   decorative and the accessible name comes from the aria-label. */

type RailItem = { to: string; icon: string; label: string };

const RAIL_ITEMS: readonly RailItem[] = [
  { to: "/onboarding", icon: "◆", label: "总览" },
  { to: "/sync", icon: "⤳", label: "同步" },
  { to: "/tasks", icon: "◱", label: "任务中心" },
  { to: "/reviews", icon: "❖", label: "审核" },
  { to: "/outcome", icon: "◔", label: "结果" },
];

export function Rail() {
  return (
    <nav className="rail" aria-label="主导航">
      <span className="rail-brand" aria-hidden="true">
        AG
      </span>
      {RAIL_ITEMS.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          className={({ isActive }) => (isActive ? "rail-item active" : "rail-item")}
          aria-label={item.label}
        >
          <span aria-hidden="true">{item.icon}</span>
          <span className="rail-label">{item.label}</span>
        </NavLink>
      ))}
      <span className="rail-spacer" />
      <NavLink
        to="/agents"
        className={({ isActive }) => (isActive ? "rail-item active" : "rail-item")}
        aria-label="Agents"
      >
        <span aria-hidden="true">◉</span>
        <span className="rail-label">Agents</span>
      </NavLink>
      <span className="rail-item" aria-hidden="true">
        ⚙
      </span>
    </nav>
  );
}
