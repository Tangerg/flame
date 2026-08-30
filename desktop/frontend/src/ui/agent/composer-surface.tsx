import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";

// ONE edge mechanism, and it lives entirely in `--shadow-composer-depth`: a style wanting
// a drawn border spells it there and nothing here changes.
// Carries no padding — the footer sits flush to the card's edges, so shared padding here
// would push it inward.
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
