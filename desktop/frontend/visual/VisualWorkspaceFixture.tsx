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
import * as stylex from "@stylexjs/stylex";
import { fx } from "./fixtureStyles";
import { type as typeStep } from "@/styles/tokens.stylex";

const STATE_LABELS: Record<VisualWorkspaceState, string> = {
  "dock-light": "Plan workspace",
  "dock-review": "Diff review",
  "dock-inbox": "Inbox",
  "dock-timeline": "Timeline",
  "dock-runs": "Run tree",
  "dock-explorer": "Explorer",
  "dock-search": "Search",
  "dock-skill-proposals": "Skill proposals",
  "dock-skill-library": "Skill library",
  "dock-recipes": "Recipes",
  "dock-agent-docs": "Agent docs",
  "dock-skills": "Skills",
  "dock-knowledge": "Knowledge",
  "dock-agent-memory": "Agent memory",
  "dock-feature-off": "Features off",
  "dock-notifications": "Notifications",
  "dock-tools": "Tool catalog",
  "dock-file": "File viewer",
  "dock-empty": "Diff · empty",
  "dock-catalog": "Dock catalogue",
  "dock-loading": "Diff · loading",
  "dock-error": "Diff · error",
  "full-view": "Full view",
  settings: "Settings",
};

function WorkspaceStateSidebar({ state }: { state: VisualWorkspaceState }) {
  const listRef = useRef<HTMLDivElement>(null);
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
    <div data-fixture-chrome="" {...stylex.props(fx.pane)}>
      <AgentSurfaceHeader corner="drawer" divider={false} />
      <div ref={listRef} {...stylex.props(fx.scroller)}>
        <span {...stylex.props(fx.listHead, typeStep.uiMd)}>Workspace states</span>
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
      <div {...stylex.props(fx.footGap)} />
      <div {...stylex.props(fx.listFoot, typeStep.uiXs)}>
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
        <div {...stylex.props(fx.contents)} data-testid="workspace-state" data-state={state}>
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
