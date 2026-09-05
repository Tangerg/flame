import { useLayoutEffect, useRef } from "react";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { ChatPanel } from "@/plugins/builtin/shell/kernel/panel/ChatPanel";
import { AppToaster } from "@/plugins/builtin/shell/toaster";
import {
  useActiveWorkspaceViewId,
  useWorkspaceDock,
} from "@/plugins/builtin/workspace/public/navigation";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { AgentAppShell, AgentRow, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualWorkspaceState } from "./workspaceFixtureStates";

const STATE_LABELS: Record<VisualWorkspaceState, string> = {
  "dock-light": "Plan workspace",
  "dock-review": "Diff review",
  "dock-inbox": "Inbox",
  "dock-stats": "Tool stats",
  "dock-timeline": "Timeline",
  "dock-explorer": "Explorer",
  "dock-terminal": "Terminal",
  "dock-search": "Search",
  "dock-files": "Changed files",
  "dock-skill-proposals": "Skill proposals",
  "dock-skill-library": "Skill library",
  "dock-recipes": "Recipes",
  "dock-agent-docs": "Agent docs",
  "dock-skills": "Skills",
  "dock-knowledge": "Knowledge",
  "dock-agent-memory": "Agent memory",
  "dock-feature-off": "Features off",
  "dock-run-summary": "Run summary",
  "dock-notifications": "Notifications",
  "dock-tools": "Tool catalog",
  "dock-file": "File viewer",
  "dock-empty": "Diff · empty",
  "dock-catalog": "Dock catalogue",
  "dock-loading": "Diff · loading",
  "dock-error": "Diff · error",
  settings: "Settings",
};

function WorkspaceStateSidebar({ state }: { state: VisualWorkspaceState }) {
  const listRef = useRef<HTMLDivElement>(null);
  // Keep the state this fixture is about on screen once the list outgrows the window, and land
  // on the same pixel every time. Two things fight that: `scrollIntoView` answers in fractions,
  // and the browser clamps to a FRACTIONAL maximum — this list can scroll 18.6px — so reading
  // it back rounded to 18 on one run and 19 on the next, moving every row a pixel and blowing
  // past the goldens' tolerance. Integer offsets decide the target, flooring after the clamp
  // decides the landing, and the observer re-lands it when fonts finish and the height changes.
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const land = () => {
      const active = list.querySelector<HTMLElement>("[data-active]");
      if (!active) return;
      const top = active.offsetTop;
      const bottom = top + active.offsetHeight;
      if (top < list.scrollTop) list.scrollTop = top;
      else if (bottom > list.scrollTop + list.clientHeight) {
        list.scrollTop = bottom - list.clientHeight;
      }
      list.scrollTop = Math.floor(list.scrollTop);
    };
    land();
    const observer = new ResizeObserver(land);
    observer.observe(list);
    return () => observer.disconnect();
  }, [state]);
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Empty, the way the product's drawer header is: the sidebar control is placed at
          the window-controls gutter and the header content box starts at the same edge, so
          anything written here is painted under the control. This caption is scaffolding
          and belongs with the scaffolding below it. */}
      <AgentSurfaceHeader corner="drawer" divider={false} />
      {/* The scrollport, because this list only grows: every view a round opens adds a row, and
          a state below the fold of a 720px window is a state whose golden cannot be taken. */}
      <div ref={listRef} className="flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-2 pt-2">
        <span className="px-2 pb-1 text-ui-md font-semibold text-fg">Workspace states</span>
        {(Object.keys(STATE_LABELS) as VisualWorkspaceState[]).map((candidate) => (
          <AgentRow
            key={candidate}
            icon={
              candidate === "settings"
                ? "settings"
                : candidate.includes("error")
                  ? "alert"
                  : "panel-r"
            }
            active={candidate === state}
          >
            {STATE_LABELS[candidate]}
          </AgentRow>
        ))}
      </div>
      {/* The list takes the free height now; this is the gap above the caption, not a spring. */}
      <div className="min-h-4" />
      <div className="px-4 pb-3 text-ui-xs leading-body text-fg-faint">
        Production views · deterministic providers
      </div>
    </div>
  );
}

function WorkspaceFixtureReadout({ state }: { state: VisualWorkspaceState }) {
  const dock = useWorkspaceDock();
  const activeMainViewId = useActiveWorkspaceViewId();
  const dockWidthRatio = useDockWidth().width;

  return (
    <>
      <output className="sr-only" data-testid="requested-workspace-state">
        {state}
      </output>
      <output className="sr-only" data-testid="active-dock-view">
        {dock.activeViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="dock-open">
        {String(dock.open)}
      </output>
      <output className="sr-only" data-testid="dock-view-ids">
        {dock.viewIds.join(",")}
      </output>
      <output className="sr-only" data-testid="active-main-view">
        {activeMainViewId ?? ""}
      </output>
      <output className="sr-only" data-testid="persisted-dock-ratio">
        {dockWidthRatio}
      </output>
    </>
  );
}

export function VisualWorkspaceFixture({ state }: { state: VisualWorkspaceState }) {
  const settingsOpen = state === "settings";

  return (
    <AgentAppShell
      sidebarLabel="Workspace fixture states"
      sidebarResizeLabel="Resize the workspace fixture sidebar"
      sidebarOpen={!settingsOpen}
      sidebarWidth={SIDEBAR_DEFAULT_WIDTH_PX}
      onResize={() => undefined}
      onSidebarToggle={() => undefined}
      sidebarExpandLabel="Expand the workspace fixture sidebar"
      sidebarCollapseLabel="Collapse the workspace fixture sidebar"
      sidebar={settingsOpen ? undefined : <WorkspaceStateSidebar state={state} />}
      main={
        <div className="contents" data-testid="workspace-state" data-state={state}>
          <ChatPanel onSend={() => true} />
        </div>
      }
      overlay={
        <>
          <WorkspaceFixtureReadout state={state} />
          <AppToaster />
        </>
      }
    />
  );
}
