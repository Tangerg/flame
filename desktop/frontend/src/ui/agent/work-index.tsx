import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { ScrollArea } from "@/ui/atoms/scroll-area";

export function AgentWorkIndexBody({ children }: { children: ReactNode }) {
  return (
    <ScrollArea
      hideScrollbar
      className="agent-index-scroll px-[var(--density-navigation-gutter)] pb-5 pt-2"
    >
      <div className="flex flex-col gap-y-[var(--density-navigation-section-gap)]">{children}</div>
    </ScrollArea>
  );
}

export function AgentWorkIndexSection({ children }: { children: ReactNode }) {
  return <div className="min-w-0">{children}</div>;
}

export function AgentWorkIndexGroupList({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-[var(--density-navigation-group-gap)]", className)}>
      {children}
    </div>
  );
}

export function AgentWorkIndexFooter({ children }: { children: ReactNode }) {
  return (
    <div className="flex items-center gap-1 bg-[var(--app-drawer-surface)] px-[var(--density-navigation-gutter)] pb-2.5 pt-2">
      {children}
    </div>
  );
}
