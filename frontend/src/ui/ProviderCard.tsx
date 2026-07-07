import type { ReactNode } from "react";

export function ProviderCard({
  logo,
  name,
  note,
  permissions = [],
  disabled = false,
  children,
}: {
  logo: ReactNode;
  name: ReactNode;
  note?: ReactNode;
  permissions?: ReactNode[];
  disabled?: boolean;
  children?: ReactNode;
}) {
  return (
    <div className="provider-card" data-disabled={disabled}>
      <div className="provider-head">
        <span className="provider-logo" aria-hidden="true">
          {logo}
        </span>
        <div>
          <div className="provider-name">{name}</div>
          {note != null ? <div className="card-sub">{note}</div> : null}
        </div>
      </div>
      {permissions.length > 0 ? (
        <ul className="perm-list">
          {permissions.map((perm, index) => (
            <li key={index}>{perm}</li>
          ))}
        </ul>
      ) : null}
      {children}
    </div>
  );
}
