import type { ReactNode } from "react";
import { useLayoutEffect, useRef } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";
import { AgentSeamRail, AgentSidebar, SIDEBAR_WIDTH_PROPERTY } from "./sidebar";
import { AgentDrawerToggle } from "./surface-header";

interface AgentAppShellProps {
  sidebar?: ReactNode;
  sidebarLabel: string;
  sidebarResizeLabel: string;
  sidebarOpen: boolean;
  sidebarWidth: number;
  onResize: (width: number) => void;
  onSidebarToggle: () => void;
  sidebarExpandLabel: string;
  sidebarCollapseLabel: string;
  /** The shell's own measure, reported whenever it changes, so the caller can
   *  decide whether the window still has room for everything it is showing. */
  onShellWidth?: (shellWidth: number) => void;
  main: ReactNode;
  overlay?: ReactNode;
}

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
  onShellWidth,
  main,
  overlay,
}: AgentAppShellProps) {
  const shellRef = useRef<HTMLDivElement>(null);
  const hasSidebar = sidebar !== undefined;

  const reportWidth = useRef(onShellWidth);
  useLayoutEffect(() => {
    reportWidth.current = onShellWidth;
  });

  useLayoutEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const syncWidth = () => {
      reportWidth.current?.(shell.clientWidth);
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
