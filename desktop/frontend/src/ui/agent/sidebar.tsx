import type { ReactNode } from "react";
import { clampSidebarWidth, maxSidebarWidth, SIDEBAR_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import { ResizeHandle } from "@/ui/atoms/resize-handle";

export function AgentSidebar({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <div className="agent-drawer-gap" aria-hidden />
      <aside aria-label={label} className="agent-drawer">
        <div className="agent-drawer-surface">{children}</div>
      </aside>
    </>
  );
}

export function AgentSeamRail({
  label,
  width,
  onCommit,
}: {
  label: string;
  width: number;
  onCommit: (width: number) => void;
}) {
  return (
    <ResizeHandle
      aria-label={label}
      className="agent-seam-rail"
      edge="end"
      value={width}
      container={(rail) => rail.closest<HTMLElement>(".agent-shell")}
      property={SIDEBAR_WIDTH_PROPERTY}
      read={readSidebarWidth}
      minWidth={sidebarMinWidth}
      maxWidth={maxSidebarWidth}
      onCommit={onCommit}
      resizingAttribute="data-resizing"
    />
  );
}

export const SIDEBAR_WIDTH_PROPERTY = "--sidebar-width";

function sidebarMinWidth(): number {
  return SIDEBAR_MIN_WIDTH_PX;
}

function readSidebarWidth(shell: HTMLElement): number {
  const value = Number.parseFloat(getComputedStyle(shell).getPropertyValue(SIDEBAR_WIDTH_PROPERTY));
  return clampSidebarWidth(Number.isFinite(value) ? value : 0, shell.clientWidth);
}
