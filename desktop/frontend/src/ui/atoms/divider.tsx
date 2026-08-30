import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

export function Divider({
  icon,
  intent = "neutral",
  align = "center",
  className,
  children,
}: {
  icon?: ReactNode;
  intent?: "neutral" | "accent";
  align?: "center" | "start";
  className?: string;
  children: ReactNode;
}) {
  const rule = <span aria-hidden className="h-px flex-1 bg-divider" />;
  return (
    <div
      className={cn("my-2 flex items-center gap-3 text-ui-sm font-medium text-fg-faint", className)}
    >
      {align === "center" && rule}
      {icon && (
        <div
          className={cn(
            "grid h-4.5 w-4.5 place-items-center rounded-full bg-surface-2",
            intent === "accent" ? "text-accent" : "text-fg-faint",
          )}
        >
          {icon}
        </div>
      )}
      <span className="min-w-0 shrink-0">{children}</span>
      {rule}
    </div>
  );
}
