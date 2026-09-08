import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { SidebarPanel } from "@/plugins/builtin/sidebar/public/SidebarPanel";
import {
  useSidebarDrawer,
  useSidebarWidth,
} from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { Icon } from "@/ui";
import { AgentAppShell, AgentContentCard, AgentStatusPill, AgentSurfaceHeader } from "@/ui/agent";
import type { VisualWorkIndexState } from "./shellFixtureStates";
import * as stylex from "@stylexjs/stylex";
import { corner, fx } from "./fixtureStyles";
import { type as typeStep } from "@/styles/tokens.stylex";
import { cn } from "@/lib/classNames";

const STATE_COPY: Record<VisualWorkIndexState, { title: string; body: string }> = {
  populated: {
    title: "A compact index for active work.",
    body: "Projects, sessions, and attention states come from production application projections.",
  },
  empty: {
    title: "Start with the work, not the chrome.",
    body: "The empty state explains how a project group is created without inventing sample data.",
  },
  loading: {
    title: "The shell remains stable while work loads.",
    body: "Only the query-owned list skeleton changes; window geometry and global actions stay ready.",
  },
  error: {
    title: "Failure stays local and actionable.",
    body: "The Work Index names the failed operation and points back to the Runtime connection.",
  },
};

export function VisualShellFixture({ state }: { state: VisualWorkIndexState }) {
  const t = useT();
  const drawer = useSidebarDrawer();
  const { width, setWidth } = useSidebarWidth();
  const copy = STATE_COPY[state];

  return (
    <AgentAppShell
      sidebarLabel={t("shell.region.workIndex")}
      sidebarResizeLabel={t("sidebar.action.resize")}
      sidebarOpen={!drawer.collapsed}
      sidebarWidth={width}
      onResize={setWidth}
      onSidebarToggle={drawer.toggle}
      sidebarExpandLabel={t("sidebar.action.expand")}
      sidebarCollapseLabel={t("sidebar.action.collapse")}
      sidebar={<SidebarPanel />}
      main={
        <AgentContentCard label="Shell and Work Index visual fixture">
          <AgentSurfaceHeader corner="window">
            <span {...stylex.props(typeStep.uiMd, fx.mono, fx.faint)}>scope</span>
            <span {...stylex.props(fx.faint, typeStep.uiMd)}>/</span>
            <span {...stylex.props(fx.truncate, fx.semibold, fx.ink, typeStep.uiMd)}>
              Work Index
            </span>
            <AgentStatusPill tone={state === "error" ? "waiting" : "idle"}>{state}</AgentStatusPill>
          </AgentSurfaceHeader>
          <div className={cn("panel-scroll", stylex.props(fx.paneRow).className)}>
            <div {...stylex.props(fx.emptyBox)}>
              <span {...stylex.props(fx.emptyGlyph, corner.pill)}>
                <Icon name="spark" size="md" />
              </span>
              <h1 {...stylex.props(fx.headingLoose, typeStep.displayLg)}>{copy.title}</h1>
              <p {...stylex.props(fx.afterHeading, typeStep.uiMd)}>{copy.body}</p>
            </div>
          </div>
        </AgentContentCard>
      }
      overlay={
        <>
          <Slot name="app.overlay" />
          <output className="sr-only" data-testid="persisted-sidebar-width">
            {width}
          </output>
          <output className="sr-only" data-testid="requested-work-index-state">
            {state}
          </output>
        </>
      }
    />
  );
}
