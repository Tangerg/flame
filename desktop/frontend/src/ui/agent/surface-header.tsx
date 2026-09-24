import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui";

type AgentHeaderCorner = "window" | "drawer";

interface AgentSurfaceHeaderProps extends ComponentPropsWithoutRef<"div"> {
  divider?: boolean;
  corner?: AgentHeaderCorner;
}

export function AgentSurfaceHeader({
  divider = true,
  corner,
  className,
  children,
  ...props
}: AgentSurfaceHeaderProps) {
  return (
    <div
      {...props}
      data-window-corner={corner === "window" ? "" : undefined}
      className={cn(
        "agent-surface-header",
        corner === "drawer" && "agent-drawer-header",
        divider && "agent-surface-divider",
        className,
      )}
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
}: {
  open: boolean;
  onToggle: () => void;
  showLabel: string;
  hideLabel: string;
}) {
  return (
    <div className="agent-dock-control">
      <IconButton
        icon="panel-r"
        hoverIcon={open ? "x" : undefined}
        size="sm"
        aria-expanded={open}
        title={open ? hideLabel : showLabel}
        onClick={onToggle}
      />
    </div>
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
