import type { ReactNode } from "react";
import { useLayoutEffect, useRef } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";
import { AgentSeamRail, AgentSidebar, SIDEBAR_WIDTH_PROPERTY } from "./sidebar";
import { AgentDrawerToggle } from "./surface-header";

interface AgentAppShellProps {
  /** Omit to run without a drawer (settings takes over the window). */
  sidebar?: ReactNode;
  sidebarLabel: string;
  sidebarResizeLabel: string;
  sidebarOpen: boolean;
  /** The PERSISTED width; the rail commits new values through `onResize`. */
  sidebarWidth: number;
  onResize: (width: number) => void;
  onSidebarToggle: () => void;
  sidebarExpandLabel: string;
  sidebarCollapseLabel: string;
  main: ReactNode;
  overlay?: ReactNode;
}

// The drawer's width lives as a custom property on this element so the rail can drag it
// without a React render, and so the spacer and the panel read one number.
export function AgentAppShell({
  sidebar,
  sidebarLabel,
  sidebarResizeLabel,
  sidebarOpen,
  sidebarWidth,
  onResize,
  onSidebarToggle,
  sidebarExpandLabel,
  sidebarCollapseLabel,
  main,
  overlay,
}: AgentAppShellProps) {
  const shellRef = useRef<HTMLDivElement>(null);
  const hasSidebar = sidebar !== undefined;

  // Re-clamps the persisted preference against the window WITHOUT overwriting it, so a
  // temporarily narrow window does not lose the user's chosen width.
  useLayoutEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const syncWidth = () => {
      // The observer fires on any layout change, and a drag IS one — under load it can
      // land between two pointer-moves and snap the drawer back to the uncommitted value.
      if (shell.hasAttribute("data-resizing")) return;
      shell.style.setProperty(
        SIDEBAR_WIDTH_PROPERTY,
        `${clampSidebarWidth(sidebarWidth, shell.clientWidth)}px`,
      );
    };
    syncWidth();
    const observer = new ResizeObserver(syncWidth);
    observer.observe(shell);
    return () => observer.disconnect();
  }, [sidebarWidth]);

  return (
    <div
      ref={shellRef}
      className="agent-shell"
      data-sidebar={hasSidebar && sidebarOpen ? "expanded" : "collapsed"}
    >
      {hasSidebar && <AgentSidebar label={sidebarLabel}>{sidebar}</AgentSidebar>}
      {hasSidebar && (
        <div className="agent-window-sidebar-control">
          <AgentDrawerToggle
            collapsed={!sidebarOpen}
            onToggle={onSidebarToggle}
            expandLabel={sidebarExpandLabel}
            collapseLabel={sidebarCollapseLabel}
          />
        </div>
      )}
      <div className="agent-card-backing">
        {hasSidebar && sidebarOpen && (
          <AgentSeamRail label={sidebarResizeLabel} width={sidebarWidth} onCommit={onResize} />
        )}
        {main}
      </div>
      {overlay}
    </div>
  );
}
