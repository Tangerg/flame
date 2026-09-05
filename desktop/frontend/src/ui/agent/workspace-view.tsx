import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui/atoms/icon-button";

export function AgentWorkspaceView({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("agent-workspace-view flex min-h-0 flex-1 flex-col", className)}>
      {children}
    </div>
  );
}

export function AgentViewSplit({
  navigator,
  children,
}: {
  navigator?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
      {navigator}
    </div>
  );
}

export function AgentViewNavigatorToggle({
  open,
  onToggle,
  showLabel,
  hideLabel,
}: {
  open: boolean;
  onToggle: () => void;
  showLabel: string;
  hideLabel: string;
}) {
  return (
    <IconButton
      icon="list"
      size="sm"
      aria-pressed={open}
      title={open ? hideLabel : showLabel}
      onClick={onToggle}
      className="agent-view-navigator-toggle"
    />
  );
}

export function AgentViewNavigator({
  label,
  header,
  children,
}: {
  label: string;
  header?: ReactNode;
  children: ReactNode;
}) {
  return (
    <aside aria-label={label} className="agent-view-navigator pane-split">
      {header && <div className="agent-view-navigator-header">{header}</div>}
      {children}
    </aside>
  );
}
