import type { ReactNode } from "react";

export function PageHeader({
  title,
  sub,
  actions,
}: {
  title: ReactNode;
  sub?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="page-header">
      <div>
        <h1 className="page-title">{title}</h1>
        {sub != null ? <p className="page-sub">{sub}</p> : null}
      </div>
      {actions != null ? <div className="page-actions">{actions}</div> : null}
    </div>
  );
}
