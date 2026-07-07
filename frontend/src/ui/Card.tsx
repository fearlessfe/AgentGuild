import type { ReactNode } from "react";

type CardProps = {
  title?: ReactNode;
  sub?: ReactNode;
  head?: ReactNode;
  pad?: boolean;
  children: ReactNode;
};

export function Card({ title, sub, head, pad = true, children }: CardProps) {
  const hasHeader = title != null || head != null;
  return (
    <section className="card">
      {hasHeader ? (
        <div className="card-head">
          <div>
            {title != null ? <div className="card-title">{title}</div> : null}
            {sub != null ? <div className="card-sub">{sub}</div> : null}
          </div>
          {head}
        </div>
      ) : null}
      <div className={pad ? "card-pad" : undefined}>{children}</div>
    </section>
  );
}
