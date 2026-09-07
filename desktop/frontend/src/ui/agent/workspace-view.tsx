import * as stylex from "@stylexjs/stylex";
import type { ReactNode } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui/atoms/icon-button";

const styles = stylex.create({
  view: { display: "flex", minHeight: 0, flex: 1, flexDirection: "column" },
  row: { display: "flex", minHeight: 0, flex: 1 },
  main: { display: "flex", minHeight: 0, minWidth: 0, flex: 1, flexDirection: "column" },
});

export function AgentWorkspaceView({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("agent-workspace-view", stylex.props(styles.view).className, className)}>
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
    <div {...stylex.props(styles.row)}>
      <div {...stylex.props(styles.main)}>{children}</div>
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
