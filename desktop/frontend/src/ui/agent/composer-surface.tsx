import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

export function AgentComposerSurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "agent-composer-glass overflow-hidden rounded-composer",
        "transition-[box-shadow] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}
