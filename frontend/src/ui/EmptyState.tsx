import type { ReactNode } from "react";

export function EmptyState({
  icon = "◔",
  title,
  children,
}: {
  icon?: ReactNode;
  title: ReactNode;
  children?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="es-icon" aria-hidden="true">
        {icon}
      </span>
      <div className="es-title">{title}</div>
      {children != null ? <div>{children}</div> : null}
    </div>
  );
}
