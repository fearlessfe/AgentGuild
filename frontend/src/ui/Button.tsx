import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Link } from "react-router-dom";

type Variant = "default" | "primary" | "ghost" | "danger";

function classes(variant: Variant, block?: boolean, lg?: boolean): string {
  return [
    "btn",
    variant !== "default" ? `btn--${variant}` : "",
    block ? "btn--block" : "",
    lg ? "btn--lg" : "",
  ]
    .filter(Boolean)
    .join(" ");
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  block?: boolean;
  lg?: boolean;
  icon?: ReactNode;
};

export function Button({ variant = "default", block, lg, icon, children, className, ...rest }: ButtonProps) {
  return (
    <button className={[classes(variant, block, lg), className].filter(Boolean).join(" ")} {...rest}>
      {icon ? <span aria-hidden="true">{icon}</span> : null}
      {children}
    </button>
  );
}

type ButtonLinkProps = {
  to: string;
  variant?: Variant;
  block?: boolean;
  lg?: boolean;
  icon?: ReactNode;
  children: ReactNode;
};

export function ButtonLink({ to, variant = "default", block, lg, icon, children }: ButtonLinkProps) {
  return (
    <Link className={classes(variant, block, lg)} to={to}>
      {icon ? <span aria-hidden="true">{icon}</span> : null}
      {children}
    </Link>
  );
}
