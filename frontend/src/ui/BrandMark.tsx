import type { SVGProps } from "react";

export type BrandMarkProps = SVGProps<SVGSVGElement> & {
  size?: number;
};

/** Compact network mark used wherever AgentGuild needs a recognizable anchor. */
export function BrandMark({ size = 24, className, ...props }: BrandMarkProps) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 48 48"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      <path d="M12 32L24 12L36 32" stroke="currentColor" strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M12 32H36M24 12V32" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" opacity="0.72" />
      <path d="M15 36C20.2 39.5 27.8 39.5 33 36" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" opacity="0.5" />
      <circle cx="12" cy="32" r="3.5" fill="currentColor" />
      <circle cx="24" cy="12" r="3.5" fill="currentColor" />
      <circle cx="36" cy="32" r="3.5" fill="currentColor" />
      <circle cx="24" cy="32" r="3" fill="currentColor" />
      <circle cx="24" cy="32" r="1.15" fill="var(--brand-mark-core, currentColor)" opacity="0.8" />
    </svg>
  );
}
