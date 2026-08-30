import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui/atoms/icon-button";

// The container the tracks below measure against: the dock's resize handle changes this
// view's width without the window changing at all, so the breakpoint is a container query.
export function AgentWorkspaceView({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("agent-workspace-view flex min-h-0 flex-1 flex-col bg-canvas", className)}>
      {children}
    </div>
  );
}

/**
 * Two slots and no geometry: which track yields at which width is a fact about this shape,
 * not about diffs or file trees. Not a percentage split — the navigator wants a roughly
 * constant width while the content has a hard floor. Below the width where both fit, the
 * navigator withdraws and takes its toggle with it (container query in globals.css).
 */
export function AgentViewSplit({
  navigator,
  children,
}: {
  navigator?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="agent-view-split" data-navigator={navigator ? "" : undefined}>
      <div className="agent-view-body">{children}</div>
      {navigator}
    </div>
  );
}

// Part of the split's contract, not the view's header furniture: it must disappear on the
// same breakpoint as the navigator or it reports a state nothing on screen can reach.
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

// A shape here rather than a class a view reaches for: a boundary is drawn by whatever
// owns both sides of it, and this seam is the dock's own pane split.
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
    <aside aria-label={label} className="agent-view-navigator agent-pane-split">
      {header && <div className="agent-view-navigator-header">{header}</div>}
      {children}
    </aside>
  );
}
