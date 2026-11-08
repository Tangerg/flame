import { useLayoutEffect, useRef, useState } from "react";
import { Checkbox } from "@flame/ui";
import { Slot } from "@/extensions/host/Slot";
import { AppToaster } from "@/features/notifications/toaster/index";
import { useActiveWorkspaceViewId, useWorkspaceDock } from "@/features/workspace/public/navigation";
import {
  useDockWidth,
  useSidebarDrawer,
  useSidebarWidth,
} from "@/features/workspace/public/sidebarDrawer";
import { AgentAppShell } from "@/ui/agent/index";
import { AgentRow } from "@/ui/agent/index";
import { AgentSurfaceHeader } from "@/ui/agent/index";
import { type VisualWorkspaceState } from "./workspaceFixtureStates";
import * as stylex from "@stylexjs/stylex";
import { fx } from "./fixtureStyles";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { observationTargetName } from "../observations";
const STATE_LABELS: Record<VisualWorkspaceState, string> = {
  "dock-light": "Files workspace",
  "dock-review": "Diff review",
  "dock-trajectory": "Session trajectory",
  "dock-runs": "Run tree",
  "dock-subagents": "Subagents",
  "dock-diagnostics": "Diagnostics",
  "dock-files": "Files",
  "dock-skills": "Skills",
  "dock-skills-catalog": "Skills · catalog",
  "dock-usage": "Usage",
  "dock-schedules": "Schedules",
  "dock-schedules-models": "Schedules · model choices",
  "dock-agent-memory": "Agent memory",
  "dock-feature-off": "Features off",
  "dock-file": "File viewer",
  "dock-file-error": "File read failure",
  "dock-file-unsupported": "Unsupported file",
  "dock-empty": "Diff · empty",
  "dock-catalog": "Dock catalogue",
  "dock-loading": "Diff · loading",
  "dock-error": "Diff · error",
  "full-view": "Full view",
  settings: "Settings",
};

function WorkspaceStateSidebar({
  state,
  observeLayout,
  onObserveLayoutChange,
}: {
  state: VisualWorkspaceState;
  observeLayout: boolean;
  onObserveLayoutChange: (observe: boolean) => void;
}) {
  const listRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const land = () => {
      const target =
        list.querySelector<HTMLElement>(":focus") ??
        list.querySelector<HTMLElement>("[data-active]");
      if (!target) return;
      const bounds = target.getBoundingClientRect();
      const frame = list.getBoundingClientRect();
      const top = frame.top + list.clientTop;
      const bottom = top + list.clientHeight;
      if (bounds.top < top) list.scrollTop += bounds.top - top;
      else if (bounds.bottom > bottom) {
        list.scrollTop += bounds.bottom - bottom;
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
            revealOverflow
          >
            {STATE_LABELS[candidate]}
          </AgentRow>
        ))}
      </div>
      <div {...stylex.props(fx.footGap)} />
      <div {...stylex.props(fx.listFoot, fx.ink)}>
        <Checkbox
          checked={observeLayout}
          onCheckedChange={onObserveLayoutChange}
          label="Observe layout"
        />
      </div>
    </div>
  );
}

function WorkspaceLayoutReadout({ enabled }: { enabled: boolean }) {
  const mainViewId = useActiveWorkspaceViewId();
  const dock = useWorkspaceDock();
  const scope = mainViewId ?? dock.activeViewId;
  const output = useRef<HTMLOutputElement>(null);
  const readingOutput = useRef<HTMLOutputElement>(null);
  useLayoutEffect(() => {
    const node = output.current;
    const readingNode = readingOutput.current;
    if (!node || !readingNode) return;
    node.textContent = "";
    readingNode.textContent = "";
    if (!enabled) return;
    const document = node.ownerDocument;
    const view = document.defaultView!;
    const state = {
      frame: 0,
      reader: null as HTMLElement | null,
      reading: null as Element | null,
      observedReader: null as Element | null,
      observedControls: null as Element | null,
      observedBody: null as Element | null,
      blur: null as { target: string | null; related: string | null; focused: boolean } | null,
    };
    const rect = (element: Element | null) => element?.getBoundingClientRect().toJSON() ?? null;
    const capture = () => {
      state.frame = 0;
      const reader =
        Array.from(
          document.querySelectorAll<HTMLElement>('[data-slot="workspace-view-body"]'),
        ).find(
          (candidate) =>
            !candidate.closest('[inert], [aria-hidden="true"]') &&
            view.getComputedStyle(candidate).visibility !== "hidden",
        ) ?? (state.reader?.isConnected ? state.reader : null);
      state.reader = reader;
      const active = document.activeElement;
      if (
        active &&
        reader?.contains(active) &&
        active.closest('[data-slot="panel-body"]') &&
        (active.matches('button, a[href], input, select, textarea, [role="button"]') ||
          active.hasAttribute("aria-label"))
      ) {
        state.reading = active;
      }
      if (!state.reading?.isConnected || !reader?.contains(state.reading)) state.reading = null;
      const reading = state.reading;
      const controls = reader?.querySelector('[data-slot="panel-controls"]') ?? null;
      // Content can reflow inside an unchanged viewport.
      const body = reader?.querySelector('[data-slot="panel-body"]') ?? null;
      for (const [before, next] of [
        [state.observedReader, reader],
        [state.observedControls, controls],
        [state.observedBody, body],
      ]) {
        if (before === next) continue;
        if (before) observer.unobserve(before);
        if (next) observer.observe(next);
      }
      state.observedReader = reader;
      state.observedControls = controls;
      state.observedBody = body;
      const dock = reader?.closest(".agent-context-dock") ?? null;
      const row = dock?.closest(".agent-dock-row") ?? null;
      const snapshot = {
        view: scope,
        window: {
          width: view.innerWidth,
          height: view.innerHeight,
          documentWidth: document.documentElement.clientWidth,
          dpr: view.devicePixelRatio,
          focused: document.hasFocus(),
        },
        visualViewport: view.visualViewport
          ? {
              width: view.visualViewport.width,
              height: view.visualViewport.height,
              scale: view.visualViewport.scale,
            }
          : null,
        dock: {
          state: row?.getAttribute("data-dock"),
          presentation: row?.getAttribute("data-presentation"),
          rect: rect(dock),
        },
        viewport: reader
          ? {
              rect: rect(reader),
              width: reader.clientWidth,
              height: reader.clientHeight,
              top: reader.scrollTop,
              total: reader.scrollHeight,
              fontSize: view.getComputedStyle(reader).fontSize,
            }
          : null,
        controls: rect(controls),
        focus: active ? { tag: active.localName, label: active.getAttribute("aria-label") } : null,
        blur: state.blur,
        reading: reading ? { name: observationTargetName(reading), rect: rect(reading) } : null,
      };
      node.textContent = JSON.stringify(snapshot);
      // Native accessibility snapshots bound each text node independently.
      readingNode.textContent = JSON.stringify({
        focused: snapshot.window.focused,
        focus: snapshot.focus,
        blur: snapshot.blur,
        window: [snapshot.window.width, snapshot.window.height],
        dock: snapshot.dock.state,
        viewport: snapshot.viewport
          ? [
              snapshot.viewport.width,
              snapshot.viewport.height,
              snapshot.viewport.top,
              snapshot.viewport.total,
            ]
          : null,
        reading: snapshot.reading
          ? {
              name: snapshot.reading.name,
              top: snapshot.reading.rect.top,
              bottom: snapshot.reading.rect.bottom,
            }
          : null,
      });
    };
    const schedule = () => {
      if (!state.frame) state.frame = view.requestAnimationFrame(capture);
    };
    const onBlur = (event: FocusEvent) => {
      state.blur = {
        target: event.target instanceof Element ? event.target.localName : null,
        related: event.relatedTarget instanceof Element ? event.relatedTarget.localName : null,
        focused: document.hasFocus(),
      };
      schedule();
    };
    const observer = new ResizeObserver(schedule);
    observer.observe(document.documentElement);
    document.addEventListener("focusin", schedule);
    document.addEventListener("focusout", onBlur);
    document.addEventListener("click", schedule, true);
    document.addEventListener("scroll", schedule, { capture: true, passive: true });
    view.addEventListener("resize", schedule);
    view.addEventListener("blur", schedule);
    view.addEventListener("focus", schedule);
    view.visualViewport?.addEventListener("resize", schedule);
    schedule();
    return () => {
      observer.disconnect();
      if (state.frame) view.cancelAnimationFrame(state.frame);
      document.removeEventListener("focusin", schedule);
      document.removeEventListener("focusout", onBlur);
      document.removeEventListener("click", schedule, true);
      document.removeEventListener("scroll", schedule, true);
      view.removeEventListener("resize", schedule);
      view.removeEventListener("blur", schedule);
      view.removeEventListener("focus", schedule);
      view.visualViewport?.removeEventListener("resize", schedule);
      node.textContent = "";
      readingNode.textContent = "";
    };
  }, [enabled, scope]);
  return (
    <>
      <output
        ref={output}
        className="sr-only"
        aria-label="Workspace layout observation"
        aria-live="off"
        data-testid="workspace-layout-observation"
      />
      <output
        ref={readingOutput}
        className="sr-only"
        aria-label="Workspace reading observation"
        aria-live="off"
        data-testid="workspace-reading-observation"
      />
    </>
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
  const [observeLayout, setObserveLayout] = useState(false);
  const settingsOpen = useActiveWorkspaceViewId() === "settings";
  const sidebar = useSidebarDrawer();
  const sidebarWidth = useSidebarWidth();

  return (
    <AgentAppShell
      sidebarLabel="Workspace fixture states"
      sidebarResizeLabel="Resize the workspace fixture sidebar"
      sidebarOpen={!settingsOpen && !sidebar.collapsed}
      sidebarWidth={sidebarWidth.width}
      onResize={sidebarWidth.setWidth}
      onSidebarToggle={sidebar.toggle}
      sidebarExpandLabel="Expand the workspace fixture sidebar"
      sidebarCollapseLabel="Collapse the workspace fixture sidebar"
      sidebar={
        settingsOpen ? undefined : (
          <WorkspaceStateSidebar
            state={state}
            observeLayout={observeLayout}
            onObserveLayoutChange={setObserveLayout}
          />
        )
      }
      main={
        <div {...stylex.props(fx.contents)} data-testid="workspace-state" data-state={state}>
          <Slot name="app.main" />
        </div>
      }
      overlay={
        <>
          <Slot name="app.overlay" />
          <WorkspaceFixtureReadout state={state} />
          <WorkspaceLayoutReadout enabled={observeLayout} />
          <AppToaster />
        </>
      }
    />
  );
}
