import { Fragment } from "react";

export type ContextItem = {
  label: string;
  value: string;
  mono?: boolean;
};

export function ContextBar({ items }: { items: ContextItem[] }) {
  if (items.length === 0) return null;
  return (
    <div className="contextbar">
      {items.map((item, index) => (
        <Fragment key={item.label}>
          {index > 0 ? <span className="ctx-sep" aria-hidden="true" /> : null}
          <span className="ctx">
            <span className="ctx-label">{item.label}</span>
            <span className="ctx-value">{item.mono ? <code>{item.value}</code> : item.value}</span>
          </span>
        </Fragment>
      ))}
    </div>
  );
}
