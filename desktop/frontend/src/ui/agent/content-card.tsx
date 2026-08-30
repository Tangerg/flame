import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

// The shell owns region material and the single leading divider, so pages compose content
// without learning how the drawer collapses or how a visual style paints the boundary.
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
