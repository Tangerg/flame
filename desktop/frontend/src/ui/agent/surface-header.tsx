import type { ComponentPropsWithoutRef } from "react";
import { cn } from "@/lib/classNames";
import { IconButton } from "@/ui";

/** Which window corner this header has to keep clear. `window` yields to the traffic lights
 *  only while the drawer is collapsed and animates as it opens; `drawer` is the drawer's own
 *  header, permanently behind them. Two treatments, so one closed value rather than a boolean
 *  each — the pair `windowCorner drawerCorner` has no meaning. */
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
    // The toggle floats over the dock's top-right corner, so it carries its own placement:
    // the box is what centres it on the header strip and keeps it out of the drag region.
    <div className="agent-dock-control">
      <IconButton
        icon="panel-r"
        hoverIcon={open ? "x" : undefined}
        size="sm"
        aria-expanded={open}
        title={disabled ? unavailableLabel : open ? hideLabel : showLabel}
        disabled={disabled}
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
