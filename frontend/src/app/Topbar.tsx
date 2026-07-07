import { useTheme } from "./ThemeProvider";

/* Top application bar: brand, current-module label, command search, theme
   toggle, help and the user avatar. */

export function Topbar({ module }: { module: string }) {
  const { theme, toggleTheme } = useTheme();

  return (
    <header className="topbar">
      <span className="topbar-brand">AgentGuild</span>
      {module ? <span className="topbar-module">{module}</span> : null}
      <span className="topbar-spacer" />
      <label className="topbar-search">
        <span aria-hidden="true">⌕</span>
        <input type="search" placeholder="搜索任务、仓库、审核…" aria-label="搜索任务、仓库、审核" />
        <kbd>⌘K</kbd>
      </label>
      <div className="topbar-icons">
        <button
          type="button"
          className="icon-btn"
          onClick={toggleTheme}
          aria-label={theme === "dark" ? "切换到浅色主题" : "切换到深色主题"}
          aria-pressed={theme === "light"}
        >
          {theme === "dark" ? "◐" : "◑"}
        </button>
        <button type="button" className="icon-btn" aria-label="帮助">
          ?
        </button>
        <span className="avatar" aria-hidden="true">
          L
        </span>
      </div>
    </header>
  );
}
