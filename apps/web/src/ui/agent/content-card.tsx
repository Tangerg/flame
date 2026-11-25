import { type ComponentPropsWithoutRef } from "react";
import { clsx } from "clsx";
export function AgentContentCard({
  label,
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"main"> & { label: string }) {
  return (
    <main aria-label={label} {...props} className={clsx("agent-content-card", className)}>
      {children}
    </main>
  );
}
