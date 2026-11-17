import { Group, Panel, Separator } from "react-resizable-panels";
import * as stylex from "@stylexjs/stylex";
import { type ReactNode } from "react";
import { createContext, useContext, useSyncExternalStore, useState } from "react";
import { AgentSidebar } from "@/ui/agent/sidebar";
import { AgentDrawerToggle } from "@/ui/agent/surface-header";
interface AgentAppShellProps {
  sidebar?: ReactNode;
  sidebarLabel: string;
  sidebarResizeLabel: string;
  sidebarOpen: boolean;
  sidebarLayout: Readonly<Record<string, number>> | null;
  onLayoutChanged(layout: Record<string, number>): void;
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
    height: "100%",
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
  sidebarLayout,
  onLayoutChanged,
  onSidebarToggle,
  sidebarExpandLabel,
  sidebarCollapseLabel,
  main,
  overlay,
}: AgentAppShellProps) {
  const [shell, setShell] = useState<HTMLDivElement | null>(null);
  const hasSidebar = sidebar !== undefined;

  const [initial] = useState(() => sidebarLayout ?? undefined);
  const narrow = useSyncExternalStore(
    (listener) => {
      const query = matchMedia("(max-width: 760px)");
      query.addEventListener("change", listener);
      return () => query.removeEventListener("change", listener);
    },
    () => matchMedia("(max-width: 760px)").matches,
  );

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
        <Group
          orientation={narrow ? "vertical" : "horizontal"}
          defaultLayout={initial}
          onLayoutChanged={(layout, meta) => {
            if (meta.isUserInteraction) onLayoutChanged(meta.requestedLayout ?? layout);
          }}
          style={{ flex: 1, minWidth: 0, minHeight: 0 }}
        >
          {hasSidebar && (
            <Panel
              id="sidebar"
              defaultSize={narrow ? "30%" : 275}
              minSize={sidebarOpen ? (narrow ? 100 : 240) : 0}
              maxSize={sidebarOpen ? (narrow ? "40%" : 520) : 0}
            >
              <div
                inert={!sidebarOpen}
                aria-hidden={!sidebarOpen || undefined}
                style={{ height: "100%" }}
              >
                <AgentSidebar label={sidebarLabel}>{sidebar}</AgentSidebar>
              </div>
            </Panel>
          )}
          {hasSidebar && sidebarOpen && (
            <Separator aria-label={sidebarResizeLabel} className="agent-seam-rail" />
          )}
          <Panel id="main" minSize={narrow ? 120 : 440}>
            <div {...stylex.props(styles.content)}>{main}</div>
          </Panel>
        </Group>
        {overlay}
      </AgentShellContext.Provider>
    </div>
  );
}
