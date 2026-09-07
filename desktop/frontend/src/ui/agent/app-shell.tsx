import * as stylex from "@stylexjs/stylex";
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
  main: ReactNode;
  overlay?: ReactNode;
}

const styles = stylex.create({
  // The card layer, so the drawer's cast lands behind the content rather than on top of it.
  content: {
    position: "relative",
    display: "flex",
    height: "100vh",
    minHeight: 0,
    minWidth: 0,
    flex: 1,
    zIndex: "var(--layer-card)",
  },
});

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

  useLayoutEffect(() => {
    const shell = shellRef.current;
    if (!shell) return;
    const syncWidth = () => {
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
      <div {...stylex.props(styles.content)}>
        {hasSidebar && sidebarOpen && (
          <AgentSeamRail label={sidebarResizeLabel} width={sidebarWidth} onCommit={onResize} />
        )}
        {main}
      </div>
      {overlay}
    </div>
  );
}
