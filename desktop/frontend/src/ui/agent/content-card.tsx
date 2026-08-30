import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

export function AgentContentCard({
  label,
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"main"> & { label: string }) {
  return (
    <main aria-label={label} {...props} className={cn("agent-content-card", className)}>
      {children}
    </main>
  );
}
