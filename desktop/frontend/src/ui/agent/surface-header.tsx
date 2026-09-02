import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui";

interface AgentSurfaceHeaderProps extends ComponentPropsWithoutRef<"div"> {
  divider?: boolean;
  windowCorner?: boolean;
}

export function AgentSurfaceHeader({
  divider = true,
  windowCorner,
  className,
  children,
  ...props
}: AgentSurfaceHeaderProps) {
  return (
    <div
      {...props}
      data-window-corner={windowCorner ? "" : undefined}
      className={cn("agent-surface-header", divider && "agent-surface-divider", className)}
    >
      {children}
    </div>
  );
}

export function AgentDockToggle({
  open,
  onToggle,
  showLabel,
  hideLabel,
  disabled,
  unavailableLabel,
}: {
  open: boolean;
  onToggle: () => void;
  showLabel: string;
  hideLabel: string;
  disabled?: boolean;
  unavailableLabel?: string;
}) {
  return (
    <IconButton
      icon="panel-r"
      hoverIcon={open ? "x" : undefined}
      size="sm"
      aria-expanded={open}
      title={disabled ? unavailableLabel : open ? hideLabel : showLabel}
      disabled={disabled}
      onClick={onToggle}
    />
  );
}

export function AgentDrawerToggle({
  collapsed,
  onToggle,
  expandLabel,
  collapseLabel,
}: {
  collapsed: boolean;
  onToggle: () => void;
  expandLabel: string;
  collapseLabel: string;
}) {
  return (
    <IconButton
      icon="panel-l"
      size="sm"
      aria-expanded={!collapsed}
      aria-label={collapsed ? expandLabel : collapseLabel}
      onClick={onToggle}
    />
  );
}
