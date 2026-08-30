import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";

export function SectionLabel({
  children,
  trailing,
  className,
}: {
  children: ReactNode;
  trailing?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex min-w-0 items-center gap-2 px-2 pb-2 pt-2 font-sans text-ui-xs font-medium leading-tight text-fg-faint",
        className,
      )}
    >
      <span className="min-w-0 truncate">{children}</span>
      {trailing != null && (
        <span className="ml-auto flex shrink-0 items-center gap-1.5 normal-case tracking-normal">
          {trailing}
        </span>
      )}
    </div>
  );
}
