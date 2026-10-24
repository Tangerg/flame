import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { createContext, useContext, useLayoutEffect, useState } from "react";
import { clampSidebarWidth } from "@/lib/shellGeometry";
import { AgentSeamRail, AgentSidebar, SIDEBAR_WIDTH_PROPERTY } from "@/ui/agent/sidebar";
import { AgentDrawerToggle } from "@/ui/agent/surface-header";
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
  content: {
    position: "relative",
    display: "flex",
    height: "100dvh",
    minHeight: 0,
    minWidth: 0,
    flex: 1,
    zIndex: "var(--layer-card)",
  },
});

const AgentShellContext = createContext<HTMLDivElement | null>(null);

export function useAgentShellElement() {
  return useContext(AgentShellContext);
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
  main,
  overlay,
}: AgentAppShellProps) {
  const [shell, setShell] = useState<HTMLDivElement | null>(null);
  const hasSidebar = sidebar !== undefined;

  useLayoutEffect(() => {
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
  }, [shell, sidebarWidth]);

  return (
    <div
      ref={setShell}
      className="agent-shell"
      data-sidebar={hasSidebar ? (sidebarOpen ? "expanded" : "collapsed") : "absent"}
    >
      <AgentShellContext.Provider value={shell}>
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
        {hasSidebar && <AgentSidebar label={sidebarLabel}>{sidebar}</AgentSidebar>}
        <div {...stylex.props(styles.content)}>
          {hasSidebar && sidebarOpen && (
            <AgentSeamRail label={sidebarResizeLabel} width={sidebarWidth} onCommit={onResize} />
          )}
          {main}
        </div>
        {overlay}
      </AgentShellContext.Provider>
    </div>
  );
}
