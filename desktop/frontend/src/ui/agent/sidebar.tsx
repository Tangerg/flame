import type { ReactNode } from "react";
import { clampSidebarWidth, maxSidebarWidth, SIDEBAR_MIN_WIDTH_PX } from "@/lib/shellGeometry";
import { ResizeHandle } from "@/ui/atoms/resize-handle";

// An in-flow spacer reserving the width plus a fixed-position panel that slides. Both read
// `--sidebar-width`, so a resize is one custom-property write and a collapse one attribute.
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

// Draws no resting line — the reading plane owns that boundary; this only strengthens the
// same coordinate on hover, focus and drag.
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
      minWidth={SIDEBAR_MIN_WIDTH_PX}
      maxWidth={maxSidebarWidth}
      onCommit={onCommit}
      resizingAttribute="data-resizing"
    />
  );
}

export const SIDEBAR_WIDTH_PROPERTY = "--sidebar-width";

function readSidebarWidth(shell: HTMLElement): number {
  const value = Number.parseFloat(getComputedStyle(shell).getPropertyValue(SIDEBAR_WIDTH_PROPERTY));
  return clampSidebarWidth(Number.isFinite(value) ? value : 0, shell.clientWidth);
}
